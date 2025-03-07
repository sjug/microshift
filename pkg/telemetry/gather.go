/*
Copyright © 2025 MicroShift Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package telemetry

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	routev1 "github.com/openshift/client-go/route/clientset/versioned/typed/route/v1"
	"github.com/openshift/microshift/pkg/config"
	"github.com/openshift/microshift/pkg/util/cryptomaterial"
	io_prometheus_client "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	kubeletMetricsCAdvisor = "https://%s:10250/metrics/cadvisor"
	kubeletMetricsResource = "https://%s:10250/metrics/resource"
	kubeletMetrics         = "https://%s:10250/metrics"
)

func makeHTTPClient() (*http.Client, error) {
	clientCertsDir := cryptomaterial.KubeAPIServerToKubeletClientCertDir(cryptomaterial.CertsDirectory(config.DataDir))
	clientCertPath := filepath.Join(clientCertsDir, cryptomaterial.ClientCertFileName)
	clientKeyPath := filepath.Join(clientCertsDir, cryptomaterial.ClientKeyFileName)
	kubeletCaPath := cryptomaterial.KubeletClientCAPath(cryptomaterial.CertsDirectory(config.DataDir))

	caCert, err := os.ReadFile(kubeletCaPath)
	if err != nil {
		return nil, fmt.Errorf("error reading CA certificate: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to add CA certificate to pool")
	}

	clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("error loading client certificate and key: %w", err)
	}

	tlsConfig := &tls.Config{
		RootCAs:      caCertPool,                    // Use the custom CA
		Certificates: []tls.Certificate{clientCert}, // Use the client certificate and key
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
	return client, nil
}

func fetchKubeletMetricsRaw(client *http.Client, url string) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading metrics: %v", err)
	}
	return data, nil
}

func aggregateMetricValues(metrics []*io_prometheus_client.Metric) float64 {
	var value float64 = 0
	for _, metric := range metrics {
		if metric.Gauge != nil {
			value += *metric.Gauge.Value
		}
		if metric.Counter != nil {
			value += *metric.Counter.Value
		}
		if metric.Untyped != nil {
			value += *metric.Untyped.Value
		}
	}
	return value
}

func filterMetricsByLabel(metrics []*io_prometheus_client.Metric, labelName string, labelValue string) []*io_prometheus_client.Metric {
	filteredMetrics := make([]*io_prometheus_client.Metric, 0)
	for _, metric := range metrics {
		for _, label := range metric.Label {
			if label.GetName() == labelName && label.GetValue() == labelValue {
				filteredMetrics = append(filteredMetrics, metric)
			}
		}
	}
	return filteredMetrics
}

func filterMetricFamiliesByName(metricFamilies map[string]*io_prometheus_client.MetricFamily, names []string) map[string]*io_prometheus_client.MetricFamily {
	filteredFamilies := make(map[string]*io_prometheus_client.MetricFamily)
	for _, name := range names {
		if data, ok := metricFamilies[name]; ok {
			filteredFamilies[name] = data
		}
	}
	return filteredFamilies
}

func fetchKubeletMetrics(cfg *config.Config) (map[string]*io_prometheus_client.MetricFamily, error) {
	client, err := makeHTTPClient()
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP client: %v", err)
	}

	metricsData := []byte{}
	kubeletEndpoints := []string{kubeletMetrics, kubeletMetricsCAdvisor, kubeletMetricsResource}
	for _, endpoint := range kubeletEndpoints {
		endpoint = fmt.Sprintf(endpoint, cfg.Node.HostnameOverride)
		data, err := fetchKubeletMetricsRaw(client, endpoint)
		if err != nil {
			return nil, fmt.Errorf("error fetching kubelet metrics from endpoint %v: %v", endpoint, err)
		}
		metricsData = append(metricsData, data...)
	}
	parser := expfmt.TextParser{}
	metricFamilies, err := parser.TextToMetricFamilies(bytes.NewReader(metricsData))
	if err != nil {
		return nil, fmt.Errorf("error parsing metrics: %v", err)
	}
	return metricFamilies, nil
}

func fetchNodeLabels(cfg *config.Config) (map[string]string, error) {
	kubeconfig := filepath.Join(cfg.KubeConfigRootAdminPath(), "kubeconfig")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("error building kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating Kubernetes clientset: %w", err)
	}
	nodes, err := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("unable to get node labels: %v", err)
	}
	labels := make(map[string]string)
	for _, node := range nodes.Items {
		for name, value := range node.Labels {
			labels[name] = value
		}
	}
	return labels, nil
}

func fetchKubernetesResources(cfg *config.Config) (map[string]int, error) {
	kubeconfig := filepath.Join(cfg.KubeConfigRootAdminPath(), "kubeconfig")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("error building kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating Kubernetes clientset: %w", err)
	}
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing pods: %w", err)
	}
	runningPods := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase == "Running" {
			runningPods++
		}
	}
	namespaces, err := clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing namespaces: %w", err)
	}
	services, err := clientset.CoreV1().Services("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing services: %w", err)
	}
	ingresses, err := clientset.NetworkingV1().Ingresses("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing ingresses: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating dynamic client: %v", err)
	}
	crdGVR := apiextensionsv1.SchemeGroupVersion.WithResource("customresourcedefinitions")
	crdList, err := dynamicClient.Resource(crdGVR).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing CRDs: %v", err)
	}

	routeClient, err := routev1.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("error creating OpenShift route client: %w", err)
	}
	routes, err := routeClient.Routes("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("error listing routes: %w", err)
	}

	metrics := map[string]int{
		"pods":                        runningPods,
		"namespaces":                  len(namespaces.Items),
		"services":                    len(services.Items),
		"ingresses.networking.k8s.io": len(ingresses.Items),
		"routes.route.openshift.io":   len(routes.Items),
		"customresourcedefinitions.apiextensions.k8s.io": len(crdList.Items),
	}
	return metrics, nil
}

func fetchOsVersionID() (string, error) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return "", fmt.Errorf("error opening /etc/os-release: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "VERSION_ID=") {
			return strings.Trim(strings.SplitN(line, "=", 2)[1], `"`), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading /etc/os-release: %w", err)
	}

	return "", fmt.Errorf("VERSION_ID not found in /etc/os-release")
}

package components

import (
	"os"

	"github.com/openshift/microshift/pkg/assets"
	"github.com/openshift/microshift/pkg/config"
	"github.com/openshift/microshift/pkg/util/cryptomaterial"
	"k8s.io/klog/v2"
)

func startServiceCAController(cfg *config.MicroshiftConfig, kubeconfigPath string) error {
	var (
		//TODO: fix the rolebinding and sa
		clusterRoleBinding = []string{
			"assets/components/service-ca/clusterrolebinding.yaml",
		}
		clusterRole = []string{
			"assets/components/service-ca/clusterrole.yaml",
		}
		roleBinding = []string{
			"assets/components/service-ca/rolebinding.yaml",
		}
		role = []string{
			"assets/components/service-ca/role.yaml",
		}
		apps = []string{
			"assets/components/service-ca/deployment.yaml",
		}
		ns = []string{
			"assets/components/service-ca/ns.yaml",
		}
		sa = []string{
			"assets/components/service-ca/sa.yaml",
		}
		secret     = "assets/components/service-ca/signing-secret.yaml"
		secretName = "signing-key"
		cm         = "assets/components/service-ca/signing-cabundle.yaml"
		cmName     = "signing-cabundle"
	)

	caPath := cryptomaterial.UltimateTrustBundlePath(cryptomaterial.CertsDirectory(cfg.DataDir))
	tlsCrtPath := cfg.DataDir + "/resources/service-ca/secrets/service-ca/tls.crt"
	tlsKeyPath := cfg.DataDir + "/resources/service-ca/secrets/service-ca/tls.key"
	cmData := map[string]string{}
	secretData := map[string][]byte{}
	cabundle, err := os.ReadFile(caPath)
	if err != nil {
		return err
	}
	tlscrt, err := os.ReadFile(tlsCrtPath)
	if err != nil {
		return err
	}
	tlskey, err := os.ReadFile(tlsKeyPath)
	if err != nil {
		return err
	}
	cmData["ca-bundle.crt"] = string(cabundle)
	secretData["tls.crt"] = tlscrt
	secretData["tls.key"] = tlskey

	if err := assets.ApplyNamespaces(ns, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply ns %v: %v", ns, err)
		return err
	}
	if err := assets.ApplyClusterRoleBindings(clusterRoleBinding, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply clusterRolebinding %v: %v", clusterRoleBinding, err)
		return err
	}
	if err := assets.ApplyClusterRoles(clusterRole, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply clusterRole %v: %v", clusterRole, err)
		return err
	}
	if err := assets.ApplyRoleBindings(roleBinding, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply rolebinding %v: %v", roleBinding, err)
		return err
	}
	if err := assets.ApplyRoles(role, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply role %v: %v", role, err)
		return err
	}
	if err := assets.ApplyServiceAccounts(sa, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply sa %v: %v", sa, err)
		return err
	}
	if err := assets.ApplySecretWithData(secret, secretData, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply secret %v: %v", secret, err)
		return err
	}
	if err := assets.ApplyConfigMapWithData(cm, cmData, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply sa %v: %v", cm, err)
		return err
	}
	extraParams := assets.RenderParams{
		"CAConfigMap": cmName,
		"TLSSecret":   secretName,
	}
	if err := assets.ApplyDeployments(apps, renderTemplate, renderParamsFromConfig(cfg, extraParams), kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply apps %v: %v", apps, err)
		return err
	}
	return nil
}

func startIngressController(cfg *config.MicroshiftConfig, kubeconfigPath string) error {
	var (
		clusterRoleBinding = []string{
			"assets/components/openshift-router/cluster-role-binding.yaml",
			"assets/components/openshift-router/ingress-to-route-controller-clusterrolebinding.yaml",
		}
		clusterRole = []string{
			"assets/components/openshift-router/cluster-role.yaml",
			"assets/components/openshift-router/ingress-to-route-controller-clusterrole.yaml",
		}
		apps = []string{
			"assets/components/openshift-router/deployment.yaml",
		}
		ns = []string{
			"assets/components/openshift-router/namespace.yaml",
		}
		sa = []string{
			"assets/components/openshift-router/service-account.yaml",
		}
		cm = []string{
			"assets/components/openshift-router/configmap.yaml",
		}
		svc = []string{
			"assets/components/openshift-router/service-internal.yaml",
		}
		extSvc = []string{
			"assets/components/openshift-router/service-cloud.yaml",
		}
	)
	if err := assets.ApplyNamespaces(ns, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply namespaces %v: %v", ns, err)
		return err
	}
	if err := assets.ApplyClusterRoles(clusterRole, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply clusterRole %v: %v", clusterRole, err)
		return err
	}
	if err := assets.ApplyClusterRoleBindings(clusterRoleBinding, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply clusterRolebinding %v: %v", clusterRoleBinding, err)
		return err
	}
	if err := assets.ApplyServiceAccounts(sa, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply serviceAccount %v %v", sa, err)
		return err
	}
	if err := assets.ApplyConfigMaps(cm, nil, nil, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply configMap %v, %v", cm, err)
		return err
	}
	if err := assets.ApplyServices(svc, nil, nil, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply service %v %v", svc, err)
		return err
	}
	if err := assets.ApplyServices(extSvc, nil, nil, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply external ingress svc %v: %v", extSvc, err)
		return err
	}
	if err := assets.ApplyDeployments(apps, renderTemplate, renderParamsFromConfig(cfg, nil), kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply apps %v: %v", apps, err)
		return err
	}
	return nil
}

func startDNSController(cfg *config.MicroshiftConfig, kubeconfigPath string) error {
	var (
		clusterRoleBinding = []string{
			"assets/components/openshift-dns/dns/cluster-role-binding.yaml",
		}
		clusterRole = []string{
			"assets/components/openshift-dns/dns/cluster-role.yaml",
		}
		apps = []string{
			"assets/components/openshift-dns/dns/daemonset.yaml",
			"assets/components/openshift-dns/node-resolver/daemonset.yaml",
		}
		ns = []string{
			"assets/components/openshift-dns/dns/namespace.yaml",
		}
		sa = []string{
			"assets/components/openshift-dns/dns/service-account.yaml",
			"assets/components/openshift-dns/node-resolver/service-account.yaml",
		}
		cm = []string{
			"assets/components/openshift-dns/dns/configmap.yaml",
		}
		svc = []string{
			"assets/components/openshift-dns/dns/service.yaml",
		}
	)
	if err := assets.ApplyNamespaces(ns, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply", "namespace", ns, "err", err)
		return err
	}
	extraParams := assets.RenderParams{
		"ClusterIP": cfg.Cluster.DNS,
	}
	if err := assets.ApplyServices(svc, renderTemplate, renderParamsFromConfig(cfg, extraParams), kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply service %v %v", svc, err)
		// service already created by coreDNS, not re-create it.
		return nil
	}
	if err := assets.ApplyClusterRoles(clusterRole, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply clusterRole %v %v", clusterRole, err)
		return err
	}
	if err := assets.ApplyClusterRoleBindings(clusterRoleBinding, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply clusterRoleBinding %v %v", clusterRoleBinding, err)
		return err
	}
	if err := assets.ApplyServiceAccounts(sa, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply serviceAccount %v %v", sa, err)
		return err
	}
	if err := assets.ApplyConfigMaps(cm, nil, nil, kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply configMap %v %v", cm, err)
		return err
	}
	if err := assets.ApplyDaemonSets(apps, renderTemplate, renderParamsFromConfig(cfg, extraParams), kubeconfigPath); err != nil {
		klog.Warningf("Failed to apply apps %v %v", apps, err)
		return err
	}
	return nil
}

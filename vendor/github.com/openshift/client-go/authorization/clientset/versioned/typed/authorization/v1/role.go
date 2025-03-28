// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	context "context"

	authorizationv1 "github.com/openshift/api/authorization/v1"
	applyconfigurationsauthorizationv1 "github.com/openshift/client-go/authorization/applyconfigurations/authorization/v1"
	scheme "github.com/openshift/client-go/authorization/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// RolesGetter has a method to return a RoleInterface.
// A group's client should implement this interface.
type RolesGetter interface {
	Roles(namespace string) RoleInterface
}

// RoleInterface has methods to work with Role resources.
type RoleInterface interface {
	Create(ctx context.Context, role *authorizationv1.Role, opts metav1.CreateOptions) (*authorizationv1.Role, error)
	Update(ctx context.Context, role *authorizationv1.Role, opts metav1.UpdateOptions) (*authorizationv1.Role, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*authorizationv1.Role, error)
	List(ctx context.Context, opts metav1.ListOptions) (*authorizationv1.RoleList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *authorizationv1.Role, err error)
	Apply(ctx context.Context, role *applyconfigurationsauthorizationv1.RoleApplyConfiguration, opts metav1.ApplyOptions) (result *authorizationv1.Role, err error)
	RoleExpansion
}

// roles implements RoleInterface
type roles struct {
	*gentype.ClientWithListAndApply[*authorizationv1.Role, *authorizationv1.RoleList, *applyconfigurationsauthorizationv1.RoleApplyConfiguration]
}

// newRoles returns a Roles
func newRoles(c *AuthorizationV1Client, namespace string) *roles {
	return &roles{
		gentype.NewClientWithListAndApply[*authorizationv1.Role, *authorizationv1.RoleList, *applyconfigurationsauthorizationv1.RoleApplyConfiguration](
			"roles",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *authorizationv1.Role { return &authorizationv1.Role{} },
			func() *authorizationv1.RoleList { return &authorizationv1.RoleList{} },
		),
	}
}

package adapter

import (
	"os"

	"github.com/layer5io/meshkit/models"
	mesherykube "github.com/layer5io/meshkit/utils/kubernetes"
	yaml "gopkg.in/yaml.v2"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Instantiates clients used in deploying and managing mesh instances, e.g. Kubernetes clients.
// This needs to be called before applying operations.
func (h *Adapter) CreateInstance(kubeconfig []byte, contextName string, ch *chan interface{}) error {
	err := h.validateKubeconfig(kubeconfig)
	if err != nil {
		return ErrCreateInstance(err)
	}

	err = h.createKubeClient(kubeconfig)
	if err != nil {
		return ErrCreateInstance(err)
	}

	err = h.createKubeconfig(kubeconfig)
	if err != nil {
		return ErrCreateInstance(err)
	}

	err = h.createMesheryKubeclient(kubeconfig)
	if err != nil {
		return ErrCreateInstance(err)
	}

	h.ClientcmdConfig.CurrentContext = contextName
	h.Channel = ch

	return nil
}

func (h *Adapter) createKubeClient(kubeconfig []byte) error {
	var (
		restConfig *rest.Config
		err        error
	)

	if len(kubeconfig) > 0 {
		restConfig, err = clientcmd.RESTConfigFromKubeConfig(kubeconfig)
		if err != nil {
			return ErrClientSet(err)
		}
	} else {
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return ErrClientSet(err)
		}
	}

	// To perform operations faster
	restConfig.QPS = float32(50)
	restConfig.Burst = int(100)

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return ErrClientSet(err)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return ErrClientSet(err)
	}

	h.KubeClient = clientset
	h.DynamicKubeClient = dynamicClient
	h.RestConfig = *restConfig
	return nil
}

func (h *Adapter) validateKubeconfig(kubeconfig []byte) error {
	clientcmdConfig, err := clientcmd.Load(kubeconfig)
	if err != nil {
		return ErrValidateKubeconfig(err)
	}

	if err := filterK8sConfigAuthInfos(clientcmdConfig.AuthInfos); err != nil {
		return ErrValidateKubeconfig(err)
	}

	if err := clientcmdapi.FlattenConfig(clientcmdConfig); err != nil {
		return ErrValidateKubeconfig(err)
	}

	if err := clientcmdapi.MinifyConfig(clientcmdConfig); err != nil {
		return ErrValidateKubeconfig(err)
	}

	h.ClientcmdConfig = clientcmdConfig

	return nil
}

func (h *Adapter) createKubeconfig(kubeconfig []byte) error {
	kconfig := models.Kubeconfig{}
	err := yaml.Unmarshal(kubeconfig, &kconfig)
	if err != nil {
		return err
	}

	// To have control over what exactly to take in on kubeconfig
	h.KubeconfigHandler.SetKey("kind", kconfig.Kind)
	h.KubeconfigHandler.SetKey("apiVersion", kconfig.APIVersion)
	h.KubeconfigHandler.SetKey("current-context", kconfig.CurrentContext)
	err = h.KubeconfigHandler.SetObject("preferences", kconfig.Preferences)
	if err != nil {
		return err
	}

	err = h.KubeconfigHandler.SetObject("clusters", kconfig.Clusters)
	if err != nil {
		return err
	}

	err = h.KubeconfigHandler.SetObject("users", kconfig.Users)
	if err != nil {
		return err
	}

	err = h.KubeconfigHandler.SetObject("contexts", kconfig.Contexts)
	if err != nil {
		return err
	}

	return nil
}

func (h *Adapter) createMesheryKubeclient(kubeconfig []byte) error {
	client, err := mesherykube.New(h.KubeClient, h.RestConfig)
	if err != nil {
		return err
	}
	h.MesheryKubeclient = client
	return nil
}

// filterK8sConfigAuthInfos takes in the authInfos map and deletes any invalid
// authInfo.
//
// An authInfo is invalid if the certificate path mentioned in it is either
// invalid or is inaccessible to the adapter
//
// The function will throw an error if after filtering the authInfos it becomes
// empty which indicates that the kubeconfig cannot be used for communicating
// with the kubernetes server.
func filterK8sConfigAuthInfos(authInfos map[string]*clientcmdapi.AuthInfo) error {
	for key, authInfo := range authInfos {
		// If clientCertficateData is not present then proceed to check
		// the client certicate path
		if len(authInfo.ClientCertificateData) == 0 {
			if _, err := os.Stat(authInfo.ClientCertificate); err != nil {
				// If the path is inaccessible or invalid then delete that authinfo
				delete(authInfos, key)
			}
		}
	}

	// In the end if the authInfos map is empty then the kubeconfig is
	// invalid and cannot be used for communicating with kubernetes
	if len(authInfos) == 0 {
		return ErrAuthInfosInvalidMsg
	}

	return nil
}

package testing

// Shared literals for unstructured objects and common GVR/policy fixtures (goconst).
const (
	k8sKeyAPIVersion = "apiVersion"
	k8sKeyKind       = "kind"
	k8sKeyMetadata   = "metadata"
	k8sKeyName       = "name"
	k8sKeyNamespace  = "namespace"
	k8sKeyUID        = "uid"
	k8sKeyLabels     = "labels"
	k8sKeyCreationTS = "creationTimestamp"

	k8sAPIV1         = "v1"
	k8sKindConfigMap = "ConfigMap"

	k8sNSDefault = "default"
	k8sNSTest    = "test"

	k8sNameTestCM = "test-cm"
	k8sUIDTest    = "test-uid"

	k8sLabelKeyApp  = "app"
	k8sLabelValTest = "test"

	k8sPolicyNameTest = "test-policy"
	k8sResConfigMaps  = "configmaps"
)

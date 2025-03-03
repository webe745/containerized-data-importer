package utils

import (
	"context"
	"strconv"
	"time"

	"fmt"

	"github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	k8sv1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	cdiClientset "kubevirt.io/containerized-data-importer/pkg/client/clientset/versioned"
)

const (
	defaultPollInterval   = 2 * time.Second
	defaultPollPeriod     = 270 * time.Second
	defaultPollPeriodFast = 30 * time.Second

	// DefaultPvcMountPath is the default mount path used
	DefaultPvcMountPath = "/dev/pvc"

	// DefaultImagePath is the default destination for images created by CDI
	DefaultImagePath = DefaultPvcMountPath + "/disk.img"

	pvcPollInterval = defaultPollInterval
	pvcCreateTime   = defaultPollPeriod
	pvcDeleteTime   = defaultPollPeriod
	pvcPhaseTime    = defaultPollPeriod
)

// CreatePVCFromDefinition creates a PVC in the passed in namespace from the passed in PersistentVolumeClaim definition.
// An example of creating a PVC without annotations looks like this:
// CreatePVCFromDefinition(client, namespace, NewPVCDefinition(name, size, nil, nil))
func CreatePVCFromDefinition(clientSet *kubernetes.Clientset, namespace string, def *k8sv1.PersistentVolumeClaim) (*k8sv1.PersistentVolumeClaim, error) {
	var pvc *k8sv1.PersistentVolumeClaim
	err := wait.PollImmediate(pvcPollInterval, pvcCreateTime, func() (bool, error) {
		var err error
		pvc, err = clientSet.CoreV1().PersistentVolumeClaims(namespace).Create(context.TODO(), def, metav1.CreateOptions{})
		if err == nil || apierrs.IsAlreadyExists(err) {
			return true, nil
		}
		return false, err
	})
	if err != nil {
		return nil, err
	}
	return pvc, nil
}

// WaitForPVC waits for a PVC
func WaitForPVC(clientSet *kubernetes.Clientset, namespace, name string) (*k8sv1.PersistentVolumeClaim, error) {
	var pvc *k8sv1.PersistentVolumeClaim
	err := wait.PollImmediate(pvcPollInterval, pvcCreateTime, func() (bool, error) {
		var err error
		pvc, err = FindPVC(clientSet, namespace, name)
		if err != nil {
			if apierrs.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return pvc, nil
}

// DeletePVC deletes the passed in PVC
func DeletePVC(clientSet *kubernetes.Clientset, namespace, pvcName string) error {
	return wait.PollImmediate(pvcPollInterval, pvcDeleteTime, func() (bool, error) {
		err := clientSet.CoreV1().PersistentVolumeClaims(namespace).Delete(context.TODO(), pvcName, metav1.DeleteOptions{})
		if err == nil || apierrs.IsNotFound(err) {
			return true, nil
		}
		return false, err
	})
}

// FindPVC Finds the passed in PVC
func FindPVC(clientSet *kubernetes.Clientset, namespace, pvcName string) (*k8sv1.PersistentVolumeClaim, error) {
	return clientSet.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
}

// WaitForPVCAnnotation waits for an anotation to appear on a PVC
func WaitForPVCAnnotation(clientSet *kubernetes.Clientset, namespace string, pvc *k8sv1.PersistentVolumeClaim, annotation string) (string, bool, error) {
	var result string
	var called bool
	err := pollPVCAnnotation(clientSet, namespace, pvc, annotation, func(value string) bool {
		called = true
		result = value
		return true
	})
	return result, called, err
}

// WaitForPVCAnnotationWithValue waits for an annotation with a specific value on a PVC
func WaitForPVCAnnotationWithValue(clientSet *kubernetes.Clientset, namespace string, pvc *k8sv1.PersistentVolumeClaim, annotation, expected string) (bool, error) {
	var result bool
	err := pollPVCAnnotation(clientSet, namespace, pvc, annotation, func(value string) bool {
		result = (expected == value)
		return result
	})
	return result, err
}

// WaitPVCPodStatusRunning waits for the pod status annotation to be Running
func WaitPVCPodStatusRunning(clientSet *kubernetes.Clientset, pvc *k8sv1.PersistentVolumeClaim) (bool, error) {
	return WaitForPVCAnnotationWithValue(clientSet, pvc.Namespace, pvc, uploadStatusAnnotation, string(k8sv1.PodRunning))
}

// WaitPVCPodStatusSucceeded waits for the pod status annotation to be Succeeded
func WaitPVCPodStatusSucceeded(clientSet *kubernetes.Clientset, pvc *k8sv1.PersistentVolumeClaim) (bool, error) {
	return WaitForPVCAnnotationWithValue(clientSet, pvc.Namespace, pvc, uploadStatusAnnotation, string(k8sv1.PodSucceeded))
}

// WaitPVCPodStatusFailed waits for the pod status annotation to be Failed
func WaitPVCPodStatusFailed(clientSet *kubernetes.Clientset, pvc *k8sv1.PersistentVolumeClaim) (bool, error) {
	return WaitForPVCAnnotationWithValue(clientSet, pvc.Namespace, pvc, uploadStatusAnnotation, string(k8sv1.PodFailed))
}

// WaitPVCPodStatusReady waits for the pod ready annotation to be true
func WaitPVCPodStatusReady(clientSet *kubernetes.Clientset, pvc *k8sv1.PersistentVolumeClaim) (bool, error) {
	return WaitForPVCAnnotationWithValue(clientSet, pvc.Namespace, pvc, uploadReadyAnnotation, strconv.FormatBool(true))
}

type pollPVCAnnotationFunc = func(string) bool

func pollPVCAnnotation(clientSet *kubernetes.Clientset, namespace string, pvc *k8sv1.PersistentVolumeClaim, annotation string, f pollPVCAnnotationFunc) error {
	err := wait.PollImmediate(pvcPollInterval, pvcCreateTime, func() (bool, error) {
		pvc, err := FindPVC(clientSet, namespace, pvc.Name)
		if err != nil {
			return false, err
		}
		fmt.Fprintf(ginkgo.GinkgoWriter, "INFO: PVC annotations: %v\n", pvc.ObjectMeta.Annotations)
		value, ok := pvc.ObjectMeta.Annotations[annotation]
		if ok {
			return f(value), nil
		}
		return false, err
	})
	return err
}

// NewPVCDefinitionWithSelector creates a PVC definition.
func NewPVCDefinitionWithSelector(pvcName, size, storageClassName string, selector map[string]string, annotations, labels map[string]string) *k8sv1.PersistentVolumeClaim {
	return &k8sv1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pvcName,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: k8sv1.PersistentVolumeClaimSpec{
			AccessModes: []k8sv1.PersistentVolumeAccessMode{k8sv1.ReadWriteOnce},
			Resources: k8sv1.ResourceRequirements{
				Requests: k8sv1.ResourceList{
					k8sv1.ResourceName(k8sv1.ResourceStorage): resource.MustParse(size),
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			StorageClassName: &storageClassName,
		},
	}
}

// NewPVCDefinition creates a PVC definition using the passed in name and requested size.
// You can use the following annotation keys to request an import or clone. The values are defined in the controller package
// AnnEndpoint
// AnnSecret
// AnnCloneRequest
// You can also pass in any label you want.
func NewPVCDefinition(pvcName string, size string, annotations, labels map[string]string) *k8sv1.PersistentVolumeClaim {
	return &k8sv1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pvcName,
			Annotations: annotations,
			Labels:      labels,
		},
		Spec: k8sv1.PersistentVolumeClaimSpec{
			AccessModes: []k8sv1.PersistentVolumeAccessMode{k8sv1.ReadWriteOnce},
			Resources: k8sv1.ResourceRequirements{
				Requests: k8sv1.ResourceList{
					k8sv1.ResourceName(k8sv1.ResourceStorage): resource.MustParse(size),
				},
			},
		},
	}
}

// NewBlockPVCDefinition creates a PVC definition with volumeMode 'Block'
func NewBlockPVCDefinition(pvcName string, size string, annotations, labels map[string]string, storageClassName string) *k8sv1.PersistentVolumeClaim {
	pvcDef := NewPVCDefinition(pvcName, size, annotations, labels)
	pvcDef.Spec.StorageClassName = &storageClassName
	volumeMode := corev1.PersistentVolumeBlock
	pvcDef.Spec.VolumeMode = &volumeMode
	return pvcDef
}

// WaitForPersistentVolumeClaimPhase waits for the PVC to be in a particular phase (Pending, Bound, or Lost)
func WaitForPersistentVolumeClaimPhase(clientSet *kubernetes.Clientset, namespace string, phase k8sv1.PersistentVolumeClaimPhase, pvcName string) error {
	err := wait.PollImmediate(pvcPollInterval, pvcPhaseTime, func() (bool, error) {
		pvc, err := clientSet.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
		fmt.Fprintf(ginkgo.GinkgoWriter, "INFO: Checking PVC phase: %s\n", string(pvc.Status.Phase))
		if err != nil || pvc.Status.Phase != phase {
			return false, err
		}
		return true, nil
	})
	if err != nil {
		return fmt.Errorf("PersistentVolumeClaim %s not in phase %s within %v", pvcName, phase, pvcPhaseTime)
	}
	return nil
}

// WaitPVCDeleted polls the specified PVC until timeout or it's not found, returns true if deleted in the specified timeout period, and any errors
func WaitPVCDeleted(clientSet *kubernetes.Clientset, pvcName, namespace string, timeout time.Duration) (bool, error) {
	var result bool
	err := wait.PollImmediate(2*time.Second, timeout, func() (bool, error) {
		_, err := clientSet.CoreV1().PersistentVolumeClaims(namespace).Get(context.TODO(), pvcName, metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				result = true
				return true, nil
			}
			return false, err
		}
		return false, nil
	})
	return result, err
}

// WaitPVCDeletedByUID polls the specified PVC until timeout or it's not found, returns true if the PVC with the same UID is deleted
// in the specified timeout period, and any errors
func WaitPVCDeletedByUID(clientSet *kubernetes.Clientset, pvcSpec *k8sv1.PersistentVolumeClaim, timeout time.Duration) (bool, error) {
	var result bool
	err := wait.PollImmediate(2*time.Second, timeout, func() (bool, error) {
		pvc, err := clientSet.CoreV1().PersistentVolumeClaims(pvcSpec.Namespace).Get(context.TODO(), pvcSpec.Name, metav1.GetOptions{})
		if err != nil {
			if k8serrors.IsNotFound(err) {
				result = true
				return true, nil
			}
			return false, err
		}
		result = pvcSpec.GetUID() != pvc.GetUID()
		return result, nil
	})
	return result, err
}

func getCdiCR(clientSet *cdiClientset.Clientset) (*cdiv1.CDI, error) {
	crList, err := clientSet.CdiV1beta1().CDIs().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	if len(crList.Items) != 1 {
		return nil, fmt.Errorf("There should be a single Cdi, %d items found", len(crList.Items))
	}
	return &crList.Items[0], nil
}

func waitForCDI(clientSet *cdiClientset.Clientset, condition func(cr *cdiv1.CDI) (bool, error)) error {
	err := wait.PollImmediate(pvcPollInterval, pvcCreateTime, func() (bool, error) {
		var err error
		cr, err := getCdiCR(clientSet)
		if err != nil {
			if apierrs.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return condition(cr)
	})
	if err != nil {
		return err
	}
	return nil
}

// WaitForCDICrCloneStrategy waits for a CDI CR Clone strategy
func WaitForCDICrCloneStrategy(clientSet *cdiClientset.Clientset, cloneStrategy cdiv1.CDICloneStrategy) error {
	return waitForCDI(clientSet, func(cr *cdiv1.CDI) (bool, error) {
		return cr.Spec.CloneStrategyOverride != nil && cloneStrategy == *cr.Spec.CloneStrategyOverride, nil
	})
}

// WaitForCDICrCloneStrategyNil waits for a CDI CR strategy to be nil
func WaitForCDICrCloneStrategyNil(clientSet *cdiClientset.Clientset) error {
	return waitForCDI(clientSet, func(cr *cdiv1.CDI) (bool, error) {
		return cr.Spec.CloneStrategyOverride == nil, nil
	})
}

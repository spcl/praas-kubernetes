package resources

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"knative.dev/serving/pkg/apis/serving"
)

// PodAccessor provides access to various dimensions of pods listing
// and querying for a given bound revision.
type PodAccessor struct {
	podsLister corev1listers.PodNamespaceLister
	selector   labels.Selector
}

// NewPodAccessor creates a PodAccessor implementation that counts
// pods for a namespace/revision.
func NewPodAccessor(lister corev1listers.PodLister, namespace, revisionName string) PodAccessor {
	return PodAccessor{
		podsLister: lister.Pods(namespace),
		selector: labels.SelectorFromSet(
			labels.Set{
				serving.RevisionLabelKey: revisionName,
			},
		),
	}
}

// PodCountsByState returns number of pods for the revision grouped by their state, that is
// of interest to knative (e.g. ignoring failed or terminated pods).
func (pa PodAccessor) PodCountsByState() (ready, notReady, pending, terminating int, err error) {
	pods, err := pa.podsLister.List(pa.selector)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	for _, p := range pods {
		switch p.Status.Phase {
		case corev1.PodPending:
			pending++
			notReady++
		case corev1.PodRunning:
			if p.DeletionTimestamp != nil {
				terminating++
				notReady++
				continue
			}
			if podReady(p) {
				ready++
			} else {
				notReady++
			}
		}
	}

	return ready, notReady, pending, terminating, nil
}

// ReadyCount implements EndpointsCounter.
func (pa PodAccessor) ReadyCount() (int, error) {
	r, _, _, _, err := pa.PodCountsByState()
	return r, err
}

// NotReadyCount implements EndpointsCounter.
func (pa PodAccessor) NotReadyCount() (int, error) {
	_, nr, _, _, err := pa.PodCountsByState()
	return nr, err
}

// PodFilter provides a way to filter pods for a revision.
// Returning true, means that pod should be kept.
type PodFilter func(p *corev1.Pod) bool

// PodTransformer provides a way to do something with the pod
// that has been selected by all the filters.
// For example pod transformer may extract a field and store it in
// internal state.
type PodTransformer func(p *corev1.Pod)

// ProcessPods filters all the pods using provided pod filters and then if the pod
// is selected, applies the transformer to it.
func (pa PodAccessor) ProcessPods(pt PodTransformer, pfs ...PodFilter) error {
	pods, err := pa.podsLister.List(pa.selector)
	if err != nil {
		return err
	}
	for _, p := range pods {
		if applyFilters(p, pfs...) {
			pt(p)
		}
	}
	return nil
}

func applyFilters(p *corev1.Pod, pfs ...PodFilter) bool {
	for _, pf := range pfs {
		if !pf(p) {
			return false
		}
	}
	return true
}

type PodLister struct {
	podsLister corev1listers.PodNamespaceLister
	selector   labels.Selector
}

func NewPodLister(lister corev1listers.PodLister, namespace, revisionName string) PodLister {
	return PodLister{
		podsLister: lister.Pods(namespace),
		selector: labels.SelectorFromSet(
			labels.Set{
				serving.RevisionLabelKey: revisionName,
			},
		),
	}
}

func (c *PodLister) podsSplitByAge(window time.Duration, now time.Time) (older, younger []*corev1.Pod, err error) {
	pods, err := c.podsLister.List(c.selector)
	if err != nil {
		return nil, nil, err
	}

	for _, p := range pods {
		// Make sure pod is ready
		if !podReady(p) {
			continue
		}

		// Make sure pod is running
		if !podRunning(p) {
			continue
		}

		if now.Sub(p.Status.StartTime.Time) > window {
			older = append(older, p)
		} else {
			younger = append(younger, p)
		}
	}

	return older, younger, nil
}

func (c *PodLister) PodsSplitByReady() (ready, notReady []*corev1.Pod, err error) {
	pods, err := c.podsLister.List(c.selector)
	if err != nil {
		return nil, nil, err
	}

	for _, p := range pods {
		// Make sure pod is running
		if p.DeletionTimestamp != nil {
			continue
		}
		// Make sure pod is ready
		if podReady(p) && p.Status.Phase == corev1.PodRunning {
			ready = append(ready, p)
		} else {
			notReady = append(notReady, p)
		}
	}

	return ready, notReady, nil
}

func (c *PodLister) Pods() (pods []*corev1.Pod, err error) {
	selectedPods, err := c.podsLister.List(c.selector)
	if err != nil {
		return nil, err
	}

	for _, p := range selectedPods {
		if p.DeletionTimestamp == nil &&
			(p.Status.Phase == corev1.PodRunning || p.Status.Phase == corev1.PodPending) {
			pods = append(pods, p)
		}
	}

	return pods, nil
}

func (c *PodLister) PodCount() (int, error) {
	selectedPods, err := c.podsLister.List(c.selector)
	if err != nil {
		return -1, err
	}

	count := 0
	for _, p := range selectedPods {
		if p.DeletionTimestamp == nil &&
			(p.Status.Phase == corev1.PodRunning || p.Status.Phase == corev1.PodPending) {
			count++
		}
	}

	return count, nil
}

func podRunning(p *corev1.Pod) bool {
	return p.Status.Phase == corev1.PodRunning && p.DeletionTimestamp == nil
}

// podReady checks whether pod's Ready status is True.
func podReady(p *corev1.Pod) bool {
	for _, cond := range p.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	// No ready status, probably not even running.
	return false
}

package hub

import (
	"fmt"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStoreConcurrency(t *testing.T) {
	store := NewStore()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store.UpsertPod(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("wk-%d", i),
				Namespace: "kubeshark",
			}})
			_ = store.Pods()
			store.SetLicense(fmt.Sprintf("KEY-%d", i))
			_ = store.License()
		}(i)
	}
	wg.Wait()

	if got := store.License(); got == "" {
		t.Error("store.License() empty after concurrent writes")
	}
}

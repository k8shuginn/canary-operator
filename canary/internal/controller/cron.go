package controller

import (
	"context"
	"github.com/k8shuginn/canary-operator/api/v1alpha1"
	cronv3 "github.com/robfig/cron/v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

type cronInfo struct {
	schedule string
	old, new string
	id       cronv3.EntryID
}

type Cron struct {
	client.Client
	cr    *cronv3.Cron
	idMap map[string]cronInfo
}

func NewCron(client client.Client) *Cron {
	cr := cronv3.New()
	c := &Cron{
		Client: client,
		cr:     cr,
		idMap:  make(map[string]cronInfo),
	}
	c.cr.Start()

	return c
}

func (c *Cron) Apply(
	namespace, name, spec string,
	old, new string,
) error {
	idx := makeIndex(namespace, name)
	if info, ok := c.idMap[idx]; ok {
		if info.schedule == spec && info.old == old && info.new == new {
			return nil
		}
		c.cr.Remove(info.id)
		delete(c.idMap, idx)
	}

	id, err := c.cr.AddFunc(spec, func() {
		ctx := context.Background()
		logger := log.FromContext(ctx)

		canary := &v1alpha1.Canary{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, canary); err != nil {
			logger.Error(err, "[Cron] Failed to get Canary")
			return
		}

		if canary.Status.CurrentStep < canary.Spec.TotalReplicas/canary.Spec.StepReplicas {
			canary.Status.CurrentStep++
			_ = c.Status().Update(ctx, canary)

			canary.Annotations[AnnotationLastUpdate] = time.Now().Format(time.RFC3339)
			if err := c.Update(context.Background(), canary); err != nil {
				logger.Error(err, "[Cron] Failed to update Canary")
				return
			}
		}
		logger.Info("[Cron] Updated Canary", "namespace", namespace, "name", name)
	})
	if err != nil {
		return err
	}

	c.idMap[idx] = cronInfo{
		schedule: spec,
		id:       id,
		old:      old,
		new:      new,
	}
	return nil
}

func (c *Cron) Delete(namespace, name string) {
	idx := makeIndex(namespace, name)
	if info, ok := c.idMap[idx]; ok {
		c.cr.Remove(info.id)
		delete(c.idMap, idx)
	}
}

func makeIndex(namespace, name string) string {
	return namespace + "/" + name
}

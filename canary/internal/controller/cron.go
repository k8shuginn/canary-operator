package controller

import (
	"context"
	"github.com/k8shuginn/canary-operator/api/v1alpha1"
	cronv3 "github.com/robfig/cron/v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

var (
	k8sClient client.Client
)

type CronJob struct {
	id        cronv3.EntryID
	schedule  string
	namespace string
	name      string

	old, new string
}

func (j *CronJob) Run() {
	ctx := context.Background()
	logger := log.FromContext(ctx)

	canary := &v1alpha1.Canary{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: j.namespace, Name: j.name}, canary); err != nil {
		logger.Error(err, "[Cron] Failed to get Canary")
		return
	}

	if canary.Status.CurrentStep < canary.Spec.TotalReplicas/canary.Spec.StepReplicas {
		canary.Status.CurrentStep++
		_ = k8sClient.Status().Update(ctx, canary)

		canary.Annotations[AnnotationLastUpdate] = time.Now().Format(time.RFC3339)
		if err := k8sClient.Update(context.Background(), canary); err != nil {
			logger.Error(err, "[Cron] Failed to update Canary")
			return
		}
	}
	logger.Info("[Cron] Updated Canary", "namespace", j.namespace, "name", j.name)
}

type Cron struct {
	client.Client
	cr    *cronv3.Cron
	idMap map[string]*CronJob
}

func NewCron(client client.Client) *Cron {
	cr := cronv3.New()
	c := &Cron{
		Client: client,
		cr:     cr,
		idMap:  make(map[string]*CronJob),
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

	cj := &CronJob{
		schedule:  spec,
		namespace: namespace,
		name:      name,
		old:       old,
		new:       new,
	}

	id, err := c.cr.AddJob(spec, cj)
	if err != nil {
		return err
	}
	cj.id = id
	c.idMap[idx] = cj

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

// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package defragmentor

import (
	"context"

	"github.com/gardener/etcd-backup-restore/pkg/etcdutil"
	brtypes "github.com/gardener/etcd-backup-restore/pkg/types"
	cron "github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
)

// CallbackFunc is type decalration for callback function for defragmentor
type CallbackFunc func(ctx context.Context) (*brtypes.Snapshot, error)

// defragmentorJob implement the cron.Job for etcd defragmentation.
type defragmentorJob struct {
	ctx                  context.Context
	etcdConnectionConfig *etcdutil.EtcdConnectionConfig
	logger               *logrus.Entry
	callback             CallbackFunc
}

// NewDefragmentorJob returns the new defragmentor job.
func NewDefragmentorJob(ctx context.Context, etcdConnectionConfig *etcdutil.EtcdConnectionConfig, logger *logrus.Entry, callback CallbackFunc) cron.Job {
	return &defragmentorJob{
		ctx:                  ctx,
		etcdConnectionConfig: etcdConnectionConfig,
		logger:               logger.WithField("job", "defragmentor"),
		callback:             callback,
	}
}

func (d *defragmentorJob) Run() {
	client, err := etcdutil.GetTLSClientForEtcd(d.etcdConnectionConfig)
	if err != nil {
		d.logger.Warnf("failed to create etcd client for defragmentation")
	}
	defer client.Close()

	defragCtx, defragCancel := context.WithTimeout(d.ctx, d.etcdConnectionConfig.DefragTimeout.Duration)
	err = etcdutil.DefragmentData(defragCtx, client, d.etcdConnectionConfig.Endpoints, d.logger)
	defragCancel()
	if err != nil {
		d.logger.Warnf("Failed to defrag data with error: %v", err)
	} else {
		if d.callback != nil {
			if _, err = d.callback(d.ctx); err != nil {
				d.logger.Warnf("defragmentation callback failed with error: %v", err)
			}
		}
	}
}

// DefragDataPeriodically defragments the data directory of each etcd member.
func DefragDataPeriodically(ctx context.Context, etcdConnectionConfig *etcdutil.EtcdConnectionConfig, defragmentationSchedule cron.Schedule, callback CallbackFunc, logger *logrus.Entry) {
	defragmentorJob := NewDefragmentorJob(ctx, etcdConnectionConfig, logger, callback)
	// TODO: Sync logrus logger to cron logger
	jobRunner := cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
	jobRunner.Schedule(defragmentationSchedule, defragmentorJob)

	jobRunner.Start()

	<-ctx.Done()
	jobRunnerCtx := jobRunner.Stop()
	<-jobRunnerCtx.Done()
}

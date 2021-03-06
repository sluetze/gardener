// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logging

import (
	"context"
	"fmt"
	"time"

	"github.com/gardener/gardener/test/framework"
	"github.com/gardener/gardener/test/framework/resources/templates"

	"github.com/onsi/ginkgo"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	logsCount uint64 = 10000

	initializationTimeout           = 15 * time.Minute
	kibanaAvailableTimeout          = 10 * time.Second
	getLogsFromElasticsearchTimeout = 5 * time.Minute

	loggerDeploymentCleanupTimeout = 2 * time.Minute

	fluentBitName = "fluent-bit"
	fluentdName   = "fluentd-es"
	garden        = "garden"
	logger        = "logger"
)

var _ = ginkgo.Describe("Seed logging testing", func() {

	f := framework.NewShootFramework(nil)

	framework.CBeforeEach(func(ctx context.Context) {
		checkRequiredResources(ctx, f.SeedClient)
	}, initializationTimeout)

	f.Default().CIt("Kibana should be available", func(ctx context.Context) {
		url := fmt.Sprintf("https://k.%s.%s.%s/api/status", f.Shoot.Name, f.Project.Name, f.Seed.Spec.DNS.IngressDomain)
		loggingPassword, err := f.GetLoggingPassword(ctx)
		framework.ExpectNoError(err)

		err = framework.TestHTTPEndpointWithBasicAuth(ctx, url, framework.LoggingUserName, loggingPassword)
		framework.ExpectNoError(err)
	}, kibanaAvailableTimeout)

	f.Beta().CIt("should get container logs from elasticsearch", func(ctx context.Context) {
		ginkgo.By("Calculate expected logs count")
		search, err := f.GetElasticsearchLogs(ctx, f.ShootSeedNamespace(), logger, f.SeedClient)
		framework.ExpectNoError(err)
		expectedLogsCount := search.Hits.Total + logsCount
		f.Logger.Debugf("expected logs count is %d", expectedLogsCount)

		ginkgo.By("Deploy the logger application")
		loggerParams := struct {
			HelmDeployNamespace string
			LogsCount           uint64
		}{
			f.ShootSeedNamespace(),
			logsCount,
		}

		err = f.RenderAndDeployTemplate(ctx, f.SeedClient, templates.LoggerAppName, loggerParams)
		framework.ExpectNoError(err)

		ginkgo.By("Wait until logger application is ready")
		loggerLabels := labels.SelectorFromSet(labels.Set(map[string]string{
			"app": logger,
		}))
		err = f.WaitUntilDeploymentsWithLabelsIsReady(ctx, loggerLabels, f.ShootSeedNamespace(), f.SeedClient)
		framework.ExpectNoError(err)

		ginkgo.By("Verify elasticsearch received logger application logs")
		err = WaitUntilElasticsearchReceivesLogs(ctx, f, f.ShootSeedNamespace(), logger, expectedLogsCount, f.SeedClient)
		framework.ExpectNoError(err)
	}, getLogsFromElasticsearchTimeout, framework.WithCAfterTest(func(ctx context.Context) {
		ginkgo.By("Cleaning up logger app resources")
		loggerDeploymentToDelete := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: f.ShootSeedNamespace(),
				Name:      logger,
			},
		}
		err := framework.DeleteResource(ctx, f.SeedClient, loggerDeploymentToDelete)
		framework.ExpectNoError(err)
	}, loggerDeploymentCleanupTimeout))
})

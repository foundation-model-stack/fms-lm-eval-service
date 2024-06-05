/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package driver

import (
	"context"
	"flag"
	"testing"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	driverLog = ctrl.Log.WithName("driver-test")
)

func Test_Driver(t *testing.T) {

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)

	flag.Parse()
	log.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	driver, err := NewDriver(&DriverOption{
		Context:      context.Background(),
		JobNamespace: "fms-lm-eval-service-system",
		JobName:      "evaljob-sample",
		ConfigMap:    "configmap",
		OutputPath:   ".",
		Logger:       driverLog,
		Args:         []string{"--", "sh", "-ec", "echo \"tttttttttttttttttttt\""},
	})

	if err != nil {
		t.Errorf("Create Driver failed: %v", err)
	}

	if err := driver.Run(); err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

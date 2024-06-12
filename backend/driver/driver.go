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
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	lmevalservicev1beta1 "github.com/foundation-model-stack/fms-lm-eval-service/api/v1beta1"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	client "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(lmevalservicev1beta1.AddToScheme(scheme))
}

type DriverOption struct {
	Context      context.Context
	JobNamespace string
	JobName      string
	ConfigMap    string
	OutputPath   string
	Logger       logr.Logger
	Args         []string
}

type Driver interface {
	Run() error
}

type driverImpl struct {
	client client.Client
	job    lmevalservicev1beta1.EvalJob
	Option *DriverOption
}

func NewDriver(opt *DriverOption) (Driver, error) {
	if opt == nil {
		return nil, nil
	}

	if opt.Context == nil {
		return nil, fmt.Errorf("context is nil")
	}

	if opt.JobNamespace == "" || opt.JobName == "" {
		return nil, fmt.Errorf("JobNamespace or JobName is empty")
	}

	return &driverImpl{Option: opt}, nil
}

// Run implements Driver.
func (d *driverImpl) Run() error {
	config, err := ctrl.GetConfig()
	if err != nil {
		return err
	}

	client, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	d.client = client
	if err := d.getJob(); err != nil {
		return err
	}

	if err := d.updateStatus(lmevalservicev1beta1.RunningJobState); err != nil {
		return err
	}

	err = d.exec()

	if err := d.getJob(); err != nil {
		return err
	}

	return d.updateCompleteStatus(err)
}

func (d *driverImpl) getJob() error {
	if err := d.client.Get(d.Option.Context,
		types.NamespacedName{
			Name:      d.Option.JobName,
			Namespace: d.Option.JobNamespace,
		},
		&d.job); err != nil {

		return err
	}
	return nil
}

func (d *driverImpl) exec() error {
	// Run user program.
	var args []string
	if len(d.Option.Args) > 1 {
		args = d.Option.Args[1:]
	}

	stdout, err := os.Create(filepath.Join(d.Option.OutputPath, "stdout.log"))
	if err != nil {
		return err
	}
	bout := bufio.NewWriter(stdout)

	stderr, err := os.Create(filepath.Join(d.Option.OutputPath, "stderr.log"))
	if err != nil {
		return err
	}
	berr := bufio.NewWriter(stderr)

	defer func() {
		bout.Flush()
		stdout.Sync()
		stdout.Close()
		berr.Flush()
		stderr.Sync()
		stderr.Close()
	}()

	fmt.Printf("args:%v\n", args)
	executor := exec.Command(d.Option.Args[0], args...)
	executor.Stdin = os.Stdin
	executor.Stdout = bout
	executor.Stderr = berr

	if err := executor.Run(); err != nil {
		return err
	}
	return nil
}

func (d *driverImpl) updateStatus(state lmevalservicev1beta1.JobState) error {
	if d.job.Status.State != state {
		d.job.Status.State = state
		if err := d.client.Status().Update(d.Option.Context, &d.job); err != nil {
			d.Option.Logger.Error(err, "unable to update EvalJob.Status.State")
			return err
		}
	}
	return nil
}

func (d *driverImpl) updateCompleteStatus(err error) error {
	d.job.Status.State = lmevalservicev1beta1.CompleteJobState
	d.job.Status.Reason = lmevalservicev1beta1.SucceedReason
	if err != nil {
		d.job.Status.Reason = lmevalservicev1beta1.FailedReason
		d.job.Status.Message = err.Error()
	} else {
		// read the content of result*.json
		pattern := filepath.Join(d.Option.OutputPath, "result*.json")
		filepath.WalkDir(d.Option.OutputPath, func(path string, dir fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			// only output directory
			if path != d.Option.OutputPath && dir != nil && dir.IsDir() {
				return fs.SkipDir
			}

			if matched, _ := filepath.Match(pattern, path); matched {
				bytes, err := os.ReadFile(path)
				if err != nil {
					d.Option.Logger.Error(err, "failed to retrieve the results")
				} else {
					d.job.Status.Results = string(bytes)
				}
			}
			return nil
		})
	}
	if err := d.client.Status().Update(d.Option.Context, &d.job); err != nil {
		d.Option.Logger.Error(err, "unable to update EvalJob.Status.State to Complete")
		return err
	}
	return nil
}

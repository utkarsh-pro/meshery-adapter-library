// Copyright 2020 Layer5, Inc.
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

package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/layer5io/learn-layer5/smi-conformance/conformance"

	"github.com/layer5io/meshkit/utils"
	mesherykube "github.com/layer5io/meshkit/utils/kubernetes"
)

type SMITest struct {
	id             string
	adaptorVersion string
	adaptorName    string
	ctx            context.Context
	kclient        *mesherykube.Client
	smiAddress     string
	annotations    map[string]string
	labels         map[string]string
}

type Response struct {
	ID                string    `json:"id,omitempty"`
	Date              string    `json:"date,omitempty"`
	MeshName          string    `json:"mesh_name,omitempty"`
	MeshVersion       string    `json:"mesh_version,omitempty"`
	CasesPassed       string    `json:"cases_passed,omitempty"`
	PassingPercentage string    `json:"passing_percentage,omitempty"`
	Status            string    `json:"status,omitempty"`
	MoreDetails       []*Detail `json:"more_details,omitempty"`
}

type Detail struct {
	SmiSpecification string `json:"smi_specification,omitempty"`
	SmiVersion       string `json:"smi_version,omitempty"`
	Time             string `json:"time,omitempty"`
	Assertions       string `json:"assertions,omitempty"`
	Result           string `json:"result,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Capability       string `json:"capability,omitempty"`
	Status           string `json:"status,omitempty"`
}

// SMITestOptions describes the options for the SMI Test runner
type SMITestOptions struct {
	Ctx         context.Context
	OperationID string

	// Namespace is the namespace where the SMI conformance
	// must be installed
	//
	// Defaults to "meshery"
	Namespace string

	// Manifest is the remote location of manifest
	Manifest string

	// Labels is the standard kubernetes labels
	Labels map[string]string

	// Annotations is the standard kubernetes annotations
	Annotations map[string]string
}

// RunSMITest runs the SMI test on the adapter's service mesh
func (h *Adapter) RunSMITest(opts SMITestOptions) (Response, error) {
	adapterName := h.GetName()
	adapterVersion := h.GetVersion()
	name := "smi-conformance"

	kclient, err := mesherykube.New(h.KubeClient, h.RestConfig)
	if err != nil {
		return Response{}, ErrSmiInit(fmt.Sprintf("error creating meshery kubernetes client: %v", err))
	}

	test := &SMITest{
		ctx:            opts.Ctx,
		id:             opts.OperationID,
		adaptorName:    adapterName,
		adaptorVersion: adapterVersion,
		labels:         opts.Labels,
		annotations:    opts.Annotations,
		kclient:        kclient,
	}

	response := Response{
		ID:                test.id,
		Date:              time.Now().Format(time.RFC3339),
		MeshName:          test.adaptorName,
		MeshVersion:       test.adaptorVersion,
		CasesPassed:       "0",
		PassingPercentage: "0",
		Status:            "deploying",
	}

	if err = test.installConformanceTool(opts.Manifest, opts.Namespace); err != nil {
		response.Status = "installing"
		return response, ErrInstallSmi(err)
	}

	if err = test.connectConformanceTool(name, opts.Namespace); err != nil {
		response.Status = "connecting"
		return response, ErrConnectSmi(err)
	}

	if err = test.runConformanceTest(&response); err != nil {
		response.Status = "running"
		return response, ErrRunSmi(err)
	}

	if err = test.deleteConformanceTool(opts.Manifest, opts.Namespace); err != nil {
		response.Status = "deleting"
		return response, ErrDeleteSmi(err)
	}

	response.Status = "completed"
	return response, nil
}

// installConformanceTool installs the smi conformance tool
func (test *SMITest) installConformanceTool(smiManifest, ns string) error {
	// Fetch the meanifest
	manifest, err := utils.ReadRemoteFile(smiManifest)
	if err != nil {
		return err
	}

	if err := test.kclient.ApplyManifest([]byte(manifest), mesherykube.ApplyOptions{Namespace: ns}); err != nil {
		return err
	}

	time.Sleep(20 * time.Second) // Required for all the resources to be created

	return nil
}

// deleteConformanceTool deletes the smi conformance tool
func (test *SMITest) deleteConformanceTool(smiManifest, ns string) error {
	// Fetch the meanifest
	manifest, err := utils.ReadRemoteFile(smiManifest)
	if err != nil {
		return err
	}

	if err := test.kclient.ApplyManifest(
		[]byte(manifest),
		mesherykube.ApplyOptions{Namespace: ns, Delete: true},
	); err != nil {
		return err
	}
	return nil
}

// connectConformanceTool initiates the connection
func (test *SMITest) connectConformanceTool(name, ns string) error {
	endpoint, err := test.kclient.GetServiceEndpoint(test.ctx, name, ns)
	if err != nil {
		return err
	}

	test.smiAddress = fmt.Sprintf("%s:%d", endpoint.Address, endpoint.Port)
	return nil
}

// runConformanceTest runs the conformance test
func (test *SMITest) runConformanceTest(response *Response) error {
	cClient, err := conformance.CreateClient(context.TODO(), test.smiAddress)
	if err != nil {
		return err
	}

	result, err := cClient.CClient.RunTest(context.TODO(), &conformance.Request{
		Annotations: test.annotations,
		Labels:      test.labels,
		Meshname:    test.adaptorName,
		Meshversion: test.adaptorVersion,
	})
	if err != nil {
		return err
	}

	response.CasesPassed = result.Casespassed
	response.PassingPercentage = result.Passpercent

	details := make([]*Detail, 0)

	for _, d := range result.Details {
		details = append(details, &Detail{
			SmiSpecification: d.Smispec,
			Time:             d.Time,
			Assertions:       d.Assertions,
			Result:           d.Result,
			Reason:           d.Reason,
			Capability:       d.Capability,
			Status:           d.Status,
		})
	}

	response.MoreDetails = details

	err = cClient.Close()
	if err != nil {
		return err
	}

	return nil
}

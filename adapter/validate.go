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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/layer5io/meshery-adapter-library/status"
	"github.com/layer5io/meshkit/smi"
)

type SmiTestOptions struct {
	Ctx         context.Context
	OpID        string
	Labels      map[string]string
	Annotations map[string]string
}

func (h *Adapter) ValidateSMIConformance(opts *SmiTestOptions) error {
	e := &Event{
		Operationid: opts.OpID,
		Summary:     status.Deploying,
		Details:     "None",
	}

	test, err := smi.New(opts.Ctx, opts.OpID, h.GetVersion(), strings.ToLower(h.GetName()), h.KubeClient)
	if err != nil {
		e.Summary = "Error while creating smi-conformance tool"
		e.Details = err.Error()
		h.StreamErr(e, ErrNewSmi(err))
		return err
	}

	result, err := test.Run(opts.Labels, opts.Annotations)
	if err != nil {
		e.Summary = fmt.Sprintf("Error while %s running smi-conformance test", result.Status)
		e.Details = err.Error()
		h.StreamErr(e, ErrRunSmi(err))
		return err
	}

	e.Summary = fmt.Sprintf("Smi conformance test %s successfully", result.Status)
	jsondata, _ := json.Marshal(result)
	e.Details = string(jsondata)
	h.StreamInfo(e)

	return nil
}

package copr

import (
	"fmt"
	"strings"

	"github.com/mazzegi/log"
	"github.com/pkg/errors"
)

type Command any

type CommandResponse struct {
	Messages []string
	Errors   []error
	Data     any
}

func (cr *CommandResponse) AddMsg(pattern string, args ...any) {
	cr.Messages = append(cr.Messages, fmt.Sprintf(pattern, args...))
}

func (cr *CommandResponse) Errorf(pattern string, args ...any) {
	cr.Errors = append(cr.Errors, errors.Errorf(pattern, args...))
}

func (cr *CommandResponse) AddError(err error) {
	cr.Errors = append(cr.Errors, err)
}

func (cr CommandResponse) HasErrors() bool {
	return len(cr.Errors) > 0
}

func (cr CommandResponse) Error() error {
	if len(cr.Errors) == 0 {
		return nil
	}
	return errors.Errorf(strings.Join(cr.ErrorStrings(), "\n"))
}

func (cr CommandResponse) log() {
	for _, m := range cr.Messages {
		log.Infof("controller: %s", m)
	}
	for _, e := range cr.Errors {
		log.Errorf("controller: %v", e)
	}
}

func (cr *CommandResponse) merge(ocr CommandResponse) {
	cr.Messages = append(cr.Messages, ocr.Messages...)
	cr.Errors = append(cr.Errors, ocr.Errors...)
}

//

func (cr CommandResponse) ErrorStrings() []string {
	esl := make([]string, len(cr.Errors))
	for i, e := range cr.Errors {
		esl[i] = e.Error()
	}
	return esl
}

type (
	CommandStartAll struct {
		resultC chan CommandResponse
	}
	CommandStopAll struct {
		resultC chan CommandResponse
	}
	CommandStart struct {
		resultC chan CommandResponse
		unit    string
	}
	CommandStop struct {
		resultC chan CommandResponse
		unit    string
	}
	CommandEnable struct {
		resultC chan CommandResponse
		unit    string
	}
	CommandDisable struct {
		resultC chan CommandResponse
		unit    string
	}
	CommandDeploy struct {
		resultC chan CommandResponse
		unit    string
		dir     string
	}
)

func NewCommandStartAll() *CommandStartAll {
	return &CommandStartAll{resultC: make(chan CommandResponse)}
}

func NewCommandStopAll() *CommandStopAll {
	return &CommandStopAll{resultC: make(chan CommandResponse)}
}

func NewCommandStart(unit string) *CommandStart {
	return &CommandStart{resultC: make(chan CommandResponse), unit: unit}
}

func NewCommandStop(unit string) *CommandStop {
	return &CommandStop{resultC: make(chan CommandResponse), unit: unit}
}

func NewCommandEnable(unit string) *CommandEnable {
	return &CommandEnable{resultC: make(chan CommandResponse), unit: unit}
}

func NewCommandDisable(unit string) *CommandDisable {
	return &CommandDisable{resultC: make(chan CommandResponse), unit: unit}
}

func NewCommandDeploy(unit string, dir string) *CommandDeploy {
	return &CommandDeploy{resultC: make(chan CommandResponse), unit: unit, dir: dir}
}

// API
func (c *Controller) StartAll() CommandResponse {
	cmd := NewCommandStartAll()
	c.commandC <- cmd
	resp := <-cmd.resultC
	return resp
}

func (c *Controller) StopAll() CommandResponse {
	cmd := NewCommandStopAll()
	c.commandC <- cmd
	resp := <-cmd.resultC
	return resp
}

func (c *Controller) Start(unit string) CommandResponse {
	cmd := NewCommandStart(unit)
	c.commandC <- cmd
	resp := <-cmd.resultC
	return resp
}

func (c *Controller) Stop(unit string) CommandResponse {
	cmd := NewCommandStop(unit)
	c.commandC <- cmd
	resp := <-cmd.resultC
	return resp
}

func (c *Controller) Enable(unit string) CommandResponse {
	cmd := NewCommandEnable(unit)
	c.commandC <- cmd
	resp := <-cmd.resultC
	return resp
}

func (c *Controller) Disable(unit string) CommandResponse {
	cmd := NewCommandDisable(unit)
	c.commandC <- cmd
	resp := <-cmd.resultC
	return resp
}

func (c *Controller) Deploy(unit string, dir string) CommandResponse {
	resp := CommandResponse{}
	unit = strings.TrimSpace(unit)
	if unit == "" {
		resp.Errorf("empty unit name")
		return resp
	}
	err := ValidateUnitDir(dir)
	if err != nil {
		resp.AddError(errors.Wrapf(err, "validate unit-dir %q", dir))
		return resp
	}

	cmd := NewCommandDeploy(unit, dir)
	c.commandC <- cmd
	resp = <-cmd.resultC
	return resp
}

func (c *Controller) Stat(unit string) CommandResponse {
	var resp CommandResponse
	sd, err := c.statCache.statsDescriptor(unit)
	if err != nil {
		resp.AddError(errors.Wrapf(err, "stats-descriptor of %q", unit))
		return resp
	}
	return CommandResponse{Data: sd, Messages: []string{sd.String()}}
}

func (c *Controller) StatAll() CommandResponse {
	sds := c.statCache.allStatsDescriptors()
	resp := CommandResponse{Data: sds}
	for _, sds := range sds {
		resp.AddMsg(sds.String())
	}
	return resp
}

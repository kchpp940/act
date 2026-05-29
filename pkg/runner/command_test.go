package runner

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"

	"github.com/nektos/act/pkg/common"
	"github.com/nektos/act/pkg/model"
)

func TestSetEnv(t *testing.T) {
	a := assert.New(t)
	ctx := context.Background()
	rc := new(RunContext)
	rc.Env = map[string]string{"ACTIONS_ALLOW_UNSECURE_COMMANDS": "true"}
	handler := rc.commandHandler(ctx)

	handler("::set-env name=x::valz\n")
	a.Equal("valz", rc.Env["x"])
}

func TestSetEnvBlocked(t *testing.T) {
	a := assert.New(t)
	ctx := context.Background()
	rc := new(RunContext)
	handler := rc.commandHandler(ctx)

	handler("::set-env name=x::valz\n")
	a.Equal("", rc.Env["x"])
}

func TestSetOutput(t *testing.T) {
	a := assert.New(t)
	ctx := context.Background()
	rc := new(RunContext)
	rc.StepResults = make(map[string]*model.StepResult)
	handler := rc.commandHandler(ctx)

	rc.CurrentStep = "my-step"
	rc.StepResults[rc.CurrentStep] = &model.StepResult{
		Outputs: make(map[string]string),
	}
	handler("::set-output name=x::valz\n")
	a.Equal("valz", rc.StepResults["my-step"].Outputs["x"])

	handler("::set-output name=x::percent2%25\n")
	a.Equal("percent2%", rc.StepResults["my-step"].Outputs["x"])

	handler("::set-output name=x::percent2%25%0Atest\n")
	a.Equal("percent2%\ntest", rc.StepResults["my-step"].Outputs["x"])

	handler("::set-output name=x::percent2%25%0Atest another3%25test\n")
	a.Equal("percent2%\ntest another3%test", rc.StepResults["my-step"].Outputs["x"])

	handler("::set-output name=x%3A::percent2%25%0Atest\n")
	a.Equal("percent2%\ntest", rc.StepResults["my-step"].Outputs["x:"])

	handler("::set-output name=x%3A%2C%0A%25%0D%3A::percent2%25%0Atest\n")
	a.Equal("percent2%\ntest", rc.StepResults["my-step"].Outputs["x:,\n%\r:"])
}

func TestSetOutputDoubleEscape(t *testing.T) {
	a := assert.New(t)
	ctx := context.Background()
	rc := new(RunContext)
	rc.StepResults = make(map[string]*model.StepResult)
	handler := rc.commandHandler(ctx)

	rc.CurrentStep = "my-step"
	rc.StepResults[rc.CurrentStep] = &model.StepResult{
		Outputs: make(map[string]string),
	}

	handler("::set-output name=x::literal%250A\n")
	a.Equal("literal%0A", rc.StepResults["my-step"].Outputs["x"])

	handler("::set-output name=x::multi%0Aline%250Aencoded\n")
	a.Equal("multi\nline%0Aencoded", rc.StepResults["my-step"].Outputs["x"])

	handler("::set-output name=x::percent%25percent%25\n")
	a.Equal("percent%percent%", rc.StepResults["my-step"].Outputs["x"])
}

func TestSaveStateEscape(t *testing.T) {
	rc := &RunContext{
		CurrentStep: "step",
		StepResults: map[string]*model.StepResult{},
	}

	ctx := context.Background()

	handler := rc.commandHandler(ctx)
	handler("::save-state name=state-name::state%25value%0Awith%0Dnewlines\n")

	assert.Equal(t, "state%value\nwith\rnewlines", rc.IntraActionState["step"]["state-name"])
}

func TestAddpath(t *testing.T) {
	a := assert.New(t)
	ctx := context.Background()
	rc := new(RunContext)
	rc.Env = map[string]string{"ACTIONS_ALLOW_UNSECURE_COMMANDS": "true"}
	handler := rc.commandHandler(ctx)

	handler("::add-path::/zoo\n")
	a.Equal("/zoo", rc.ExtraPath[0])

	handler("::add-path::/boo\n")
	a.Equal("/boo", rc.ExtraPath[0])
}

func TestAddPathBlocked(t *testing.T) {
	a := assert.New(t)
	ctx := context.Background()
	rc := new(RunContext)
	handler := rc.commandHandler(ctx)

	handler("::add-path::/zoo\n")
	a.Empty(rc.ExtraPath)
}

func TestStopCommands(t *testing.T) {
	logger, hook := test.NewNullLogger()

	a := assert.New(t)
	ctx := common.WithLogger(context.Background(), logger)
	rc := new(RunContext)
	rc.Env = map[string]string{"ACTIONS_ALLOW_UNSECURE_COMMANDS": "true"}
	handler := rc.commandHandler(ctx)

	handler("::set-env name=x::valz\n")
	a.Equal("valz", rc.Env["x"])
	handler("::stop-commands::my-end-token\n")
	handler("::set-env name=x::abcd\n")
	a.Equal("valz", rc.Env["x"])
	handler("::my-end-token::\n")
	handler("::set-env name=x::abcd\n")
	a.Equal("abcd", rc.Env["x"])

	messages := make([]string, 0)
	for _, entry := range hook.AllEntries() {
		messages = append(messages, entry.Message)
	}

	a.Contains(messages, "  \U00002699  ::set-env name=x::abcd\n")
}

func TestAddpathADO(t *testing.T) {
	a := assert.New(t)
	ctx := context.Background()
	rc := new(RunContext)
	rc.Env = map[string]string{"ACTIONS_ALLOW_UNSECURE_COMMANDS": "true"}
	handler := rc.commandHandler(ctx)

	handler("##[add-path]/zoo\n")
	a.Equal("/zoo", rc.ExtraPath[0])

	handler("##[add-path]/boo\n")
	a.Equal("/boo", rc.ExtraPath[0])
}

func TestAddmask(t *testing.T) {
	logger, hook := test.NewNullLogger()

	a := assert.New(t)
	ctx := context.Background()
	loggerCtx := common.WithLogger(ctx, logger)

	rc := new(RunContext)
	handler := rc.commandHandler(loggerCtx)
	handler("::add-mask::my-secret-value\n")

	a.Equal("  \U00002699  ***", hook.LastEntry().Message)
	a.NotEqual("  \U00002699  *my-secret-value", hook.LastEntry().Message)
}

func TestAddmaskEscape(t *testing.T) {
	logger, _ := test.NewNullLogger()

	a := assert.New(t)
	ctx := context.Background()
	loggerCtx := common.WithLogger(ctx, logger)

	rc := new(RunContext)
	handler := rc.commandHandler(loggerCtx)

	handler("::add-mask::secret%25with%0Anewline\n")
	a.Equal("secret%with\nnewline", rc.Masks[0])

	handler("::add-mask::another%0D%0Acrlf\n")
	a.Equal("another\r\ncrlf", rc.Masks[1])
}

// based on https://stackoverflow.com/a/10476304
func captureOutput(t *testing.T, f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	outC := make(chan string)

	go func() {
		var buf bytes.Buffer
		_, err := io.Copy(&buf, r)
		if err != nil {
			a := assert.New(t)
			a.Fail("io.Copy failed")
		}
		outC <- buf.String()
	}()

	w.Close()
	os.Stdout = old
	out := <-outC

	return out
}

func TestAddmaskUsemask(t *testing.T) {
	rc := new(RunContext)
	rc.StepResults = make(map[string]*model.StepResult)
	rc.CurrentStep = "my-step"
	rc.StepResults[rc.CurrentStep] = &model.StepResult{
		Outputs: make(map[string]string),
	}

	a := assert.New(t)

	config := &Config{
		Secrets:         map[string]string{},
		InsecureSecrets: false,
	}

	re := captureOutput(t, func() {
		ctx := context.Background()
		ctx = WithJobLogger(ctx, "0", "testjob", config, &rc.Masks, map[string]interface{}{})

		handler := rc.commandHandler(ctx)
		handler("::add-mask::secret\n")
		handler("::set-output:: token=secret\n")
	})

	a.Equal("[testjob]   \U00002699  ***\n[testjob]   \U00002699  ::set-output:: = token=***\n", re)
}

func TestLoggerMaskingDoesNotAffectContext(t *testing.T) {
	rc := new(RunContext)
	rc.StepResults = make(map[string]*model.StepResult)
	rc.CurrentStep = "my-step"
	rc.StepResults[rc.CurrentStep] = &model.StepResult{
		Outputs: make(map[string]string),
	}
	rc.IntraActionState = map[string]map[string]string{}

	a := assert.New(t)

	config := &Config{
		Secrets:         map[string]string{"API_KEY": "sk-12345-secret"},
		InsecureSecrets: false,
	}

	masks := []string{"my-secret-token"}

	masker := valueMasker(config.InsecureSecrets, config.Secrets)

	ctx := context.Background()
	ctx = WithMasks(ctx, &masks)

	entry := &logrus.Entry{
		Message: "processing request with my-secret-token and sk-12345-secret",
		Data: logrus.Fields{
			"token": "my-secret-token",
			"key":   "sk-12345-secret",
			"nested": map[string]interface{}{
				"inner": "my-secret-token",
			},
			"list": []interface{}{"sk-12345-secret", "normal-value"},
		},
		Context: ctx,
	}

	maskedEntry := masker(entry)

	a.NotContains(maskedEntry.Message, "my-secret-token")
	a.NotContains(maskedEntry.Message, "sk-12345-secret")
	a.Contains(maskedEntry.Message, "***")

	a.NotContains(maskedEntry.Data["token"], "my-secret-token")
	a.Equal("***", maskedEntry.Data["token"])
	a.NotContains(maskedEntry.Data["key"], "sk-12345-secret")
	a.Equal("***", maskedEntry.Data["key"])

	nested, ok := maskedEntry.Data["nested"].(map[string]interface{})
	a.True(ok)
	a.Equal("***", nested["inner"])

	list, ok := maskedEntry.Data["list"].([]interface{})
	a.True(ok)
	a.Equal("***", list[0])
	a.Equal("normal-value", list[1])

	ctx = WithJobLogger(ctx, "0", "testjob", config, &masks, map[string]interface{}{})

	handler := rc.commandHandler(ctx)
	handler("::add-mask::masked-value\n")
	handler("::set-output name=api_key::sk-12345-secret\n")
	handler("::set-output name=token::my-secret-token\n")
	handler("::save-state name=auth::my-secret-token\n")

	a.Equal("sk-12345-secret", rc.StepResults["my-step"].Outputs["api_key"])
	a.Equal("my-secret-token", rc.StepResults["my-step"].Outputs["token"])
	a.Equal("my-secret-token", rc.IntraActionState["my-step"]["auth"])
}

func TestSaveState(t *testing.T) {
	rc := &RunContext{
		CurrentStep: "step",
		StepResults: map[string]*model.StepResult{},
	}

	ctx := context.Background()

	handler := rc.commandHandler(ctx)
	handler("::save-state name=state-name::state-value\n")

	assert.Equal(t, "state-value", rc.IntraActionState["step"]["state-name"])
}

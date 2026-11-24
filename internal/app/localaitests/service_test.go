package localaitests

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/app"
	"github.com/ousiassllc/nefos-inbox/internal/app/apperr"
	"github.com/ousiassllc/nefos-inbox/internal/app/apptest"
	"github.com/ousiassllc/nefos-inbox/internal/app/dto"
	"github.com/ousiassllc/nefos-inbox/internal/app/localaicore"
	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

func newSvc(t *testing.T) (*app.LocalAIService, *apptest.FakeLocalAI) {
	t.Helper()
	e := apptest.NewEnv(t)
	fake := apptest.NewFakeLocalAI("http://127.0.0.1:1/v1")
	e.Deps.LocalAI = fake
	return app.NewLocalAIService(e.Deps), fake
}

func requireAppErr(t *testing.T, code string, err error) *apperr.Error {
	t.Helper()
	require.Error(t, err)
	var ae *apperr.Error
	require.True(t, errors.As(err, &ae), "apperr.Error ではない: %v", err)
	require.Equal(t, code, ae.Code)
	return ae
}

// Runtime が無い（使えない OS・未配線）なら、どの操作も UNSUPPORTED。
func TestNilRuntimeIsUnsupported(t *testing.T) {
	e := apptest.NewEnv(t)
	e.Deps.LocalAI = nil
	svc := app.NewLocalAIService(e.Deps)
	ctx := t.Context()
	_, err := svc.GetLocalAIStatus()
	_ = requireAppErr(t, "UNSUPPORTED", err)
	_, err = svc.InstallLocalAI()
	_ = requireAppErr(t, "UNSUPPORTED", err)
	_, err = svc.PullLocalAIModel("gemma4:e2b")
	_ = requireAppErr(t, "UNSUPPORTED", err)
	_, err = svc.CancelLocalAIJob()
	_ = requireAppErr(t, "UNSUPPORTED", err)
	_, err = svc.StopLocalAI()
	_ = requireAppErr(t, "UNSUPPORTED", err)
	_, err = svc.DeleteLocalAIModel(ctx, "gemma4:e2b")
	_ = requireAppErr(t, "UNSUPPORTED", err)
	_, err = svc.RemoveLocalAI(ctx)
	_ = requireAppErr(t, "UNSUPPORTED", err)
}

func TestOperationsDelegateAndReturnStatus(t *testing.T) {
	svc, fake := newSvc(t)
	ctx := t.Context()
	st, err := svc.GetLocalAIStatus()
	require.NoError(t, err)
	assert.True(t, st.Supported)
	assert.Equal(t, localai.RuntimeVersion, st.Version)
	require.Len(t, st.Models, len(localai.Catalog))

	_, err = svc.InstallLocalAI()
	require.NoError(t, err)
	_, err = svc.PullLocalAIModel("gemma4:e4b")
	require.NoError(t, err)
	_, err = svc.CancelLocalAIJob()
	require.NoError(t, err)
	_, err = svc.StopLocalAI()
	require.NoError(t, err)
	_, err = svc.DeleteLocalAIModel(ctx, "gemma4:e4b")
	require.NoError(t, err)
	_, err = svc.RemoveLocalAI(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, fake.Installs())
	assert.Equal(t, []string{"gemma4:e4b"}, fake.Pulled)
	assert.Equal(t, 1, fake.Cancels())
	assert.Equal(t, []string{"gemma4:e4b"}, fake.Deleted)
}

func TestErrorMapping(t *testing.T) {
	svc, fake := newSvc(t)
	fake.StartInstallErr = localai.ErrUnsupported
	_, err := svc.InstallLocalAI()
	_ = requireAppErr(t, "UNSUPPORTED", err)

	fake.StartPullErr = localai.ErrUnknownModel
	_, err = svc.PullLocalAIModel("nope")
	_ = requireAppErr(t, "VALIDATION", err)

	fake.StartPullErr = fmt.Errorf("wrap: %w", localai.ErrNotInstalled)
	_, err = svc.PullLocalAIModel("gemma4:e2b")
	_ = requireAppErr(t, "AI_NOT_CONFIGURED", err)

	fake.StartInstallErr = localai.ErrBusy
	_, err = svc.InstallLocalAI()
	ae := requireAppErr(t, "VALIDATION", err)
	assert.Equal(t, "busy", ae.Details["reason"])

	fake.StartPullErr = &localai.NoSpaceError{Required: 8_600_000_000, Free: 1_200_000_000}
	_, err = svc.PullLocalAIModel("gemma4:e4b")
	ae = requireAppErr(t, "VALIDATION", err)
	assert.Equal(t, "no_space", ae.Details["reason"])
	assert.Equal(t, int64(8_600_000_000), ae.Details["requiredBytes"])
	assert.Equal(t, int64(1_200_000_000), ae.Details["freeBytes"])
	assert.Equal(t, "空き容量が足りません（必要: 約 8.6 GB・空き: 約 1.2 GB）。", ae.Message)

	fake.RemoveErr = localai.ErrBusy
	_, err = svc.RemoveLocalAI(t.Context())
	ae = requireAppErr(t, "VALIDATION", err)
	assert.Equal(t, "busy", ae.Details["reason"])

	fake.StopErr = errors.New("kill failed")
	_, err = svc.StopLocalAI()
	_ = requireAppErr(t, "AI_FAILED", err)
}

func TestToDTO(t *testing.T) {
	got := localaicore.ToDTO(localai.Status{Supported: true, Installed: true, Running: true, Version: "v1", DownloadBytes: 10})
	assert.NotNil(t, got.Models, "Models は null にしない")
	assert.Empty(t, got.Models)
	assert.Nil(t, got.Job)

	got = localaicore.ToDTO(localai.Status{
		UnsupportedReason: "メモリが足りません。", LastError: "失敗",
		Job:    &localai.Job{Kind: localai.JobPull, Model: "gemma4:e2b", Stage: localai.StagePull, Completed: 5, Total: 10},
		Models: []localai.Model{{CatalogModel: localai.Catalog[0], Installed: true}},
	})
	assert.Equal(t, &dto.LocalAIJob{Kind: "pull", Model: "gemma4:e2b", Stage: "pull", CompletedBytes: 5, TotalBytes: 10}, got.Job)
	c := localai.Catalog[0]
	assert.Equal(t, []dto.LocalAIModel{{Name: c.Name, DisplayName: c.DisplayName, Description: c.Description, License: c.License, SizeBytes: c.SizeBytes, Installed: true}}, got.Models)
	assert.Equal(t, "メモリが足りません。", got.UnsupportedReason)
	assert.Equal(t, "失敗", got.LastError)
}

package aicore

import (
	"context"
	"errors"

	"github.com/ousiassllc/nefos-inbox/internal/ai"
	"github.com/ousiassllc/nefos-inbox/internal/app/aicore/localconn"
	"github.com/ousiassllc/nefos-inbox/internal/app/apperr"
)

// classifierFor は 1 件の判定に使う Classifier と、使い終わったら呼ぶ release を返す。ローカルAIは 1 件ごとに送信先を得て作る
// （managed は Ollama を起動して保持する。release で手放す）。それ以外は準備済みのものをそのまま返す。
// ローカルAIの送信先を得られない（導入が外れた・使えない環境）ときは、どのスレッドでも同じ結果なので全体を止める。
func (r *inboxZeroRun) classifierFor(ctx context.Context) (ai.Classifier, func(), error) {
	if r.sel.Provider != providerLocal {
		return r.classifier, func() {}, nil
	}
	base, release, err := localconn.Open(ctx, r.s.d.LocalAI, r.sel.Provider, r.sel.LocalMode, r.sel.BaseURL)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) {
			r.fail(ae)
		}
		return nil, release, err
	}
	cl, err := r.s.d.Classifier(r.sel.Provider, r.sel.Model, base, r.key)
	if err != nil {
		release()
		return nil, nil, initError(err)
	}
	return cl, release, nil
}

// concurrency は同時に判定する件数。ローカルAIは 1 件ずつ（1 台の Ollama・GPU に重ねて投げると遅くなり、300 秒のタイムアウトに掛かりやすい）。
func (r *inboxZeroRun) concurrency() int {
	if r.sel.Provider == providerLocal {
		return 1
	}
	return inboxZeroConcurrency
}

// classifyFailure は AI の失敗を扱う。認証（401／403）・レート制限（429）・ChatGPT の利用上限・対象外は、
// どのスレッドでも同じ結果になるので全体を止め、それ以外は該当スレッドだけ skipped にする。
// ローカルAIのサーバーに接続できないときも、どのスレッドでも同じ結果なので全体を止める。
func (r *inboxZeroRun) classifyFailure(err error) slot {
	if r.provider == providerLocal && localconn.NetworkError(err) != nil {
		r.fail(r.s.mapProviderError(r.provider, "", err))
		return slot{}
	}
	if errors.Is(err, ai.ErrUnauthorized) || errors.Is(err, ai.ErrRateLimited) || errors.Is(err, ai.ErrUsageLimit) || errors.Is(err, ai.ErrNotEligible) {
		r.fail(r.s.mapProviderError(r.provider, "", err))
		return slot{}
	}
	return slot{skipped: true}
}

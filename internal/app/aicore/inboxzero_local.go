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

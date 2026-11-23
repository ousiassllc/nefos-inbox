package callbackpage

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func render(t *testing.T, msg string) (http.Header, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	Write(rec, msg)
	return rec.Header(), rec.Body.String()
}

func TestWriteHeadersAndEscape(t *testing.T) {
	h, body := render(t, `<script>alert(1)</script>`)
	if got := h.Get("Content-Security-Policy"); got != "default-src 'none'; style-src 'unsafe-inline'" {
		t.Errorf("CSP = %q", got)
	}
	if h.Get("Cache-Control") != "no-store" || h.Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("headers = %v", h)
	}
	if strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Errorf("msg is not escaped: %s", body)
	}
}

// 色はトークン（:root の変数）経由だけにし、ライト・ダークの両方を持つ（デザインガイド C2・C5）。
func TestPageColorsComeFromTokens(t *testing.T) {
	_, body := render(t, "x")
	if !strings.Contains(body, "@media (prefers-color-scheme:dark)") {
		t.Error("dark scheme branch is missing")
	}
	hex := regexp.MustCompile(`#[0-9a-fA-F]{3,6}\b`)
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, ":root{") || strings.HasPrefix(line, "@media") {
			continue // トークンの定義行
		}
		if hex.MatchString(line) || strings.Contains(strings.ReplaceAll(line, "srgb", ""), "rgb") { // srgb は color-mix の色空間名
			t.Errorf("hard-coded color outside token definitions: %s", line)
		}
	}
	for _, want := range []string{"background:var(--bone)", "color:var(--ink)", "background:var(--surface)", "1px solid var(--border)"} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// フォントは Sora + Noto Sans JP、サイズ・行間はガイドのスケール、見出しは SemiBold 600（デザインガイド T1・T2・T3・T4）。
func TestPageTypography(t *testing.T) {
	_, body := render(t, "x")
	for _, want := range []string{
		`font-family:Sora,"Noto Sans JP",sans-serif`,
		"h1{font-size:18px;line-height:1.7;font-weight:600;",
		"p{margin:0;font-size:16px;line-height:1.8;overflow-wrap:anywhere}",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"font-family:sans-serif", "1.2rem", "line-height:1.6"} {
		if strings.Contains(body, bad) {
			t.Errorf("unexpected %q", bad)
		}
	}
}

// カードは影なしの 1px solid var(--border)・--r-lg（12px）・--pad-card（24px）（デザインガイド S1・S3）。
func TestPageCardShape(t *testing.T) {
	_, body := render(t, "x")
	m := regexp.MustCompile(`main\{([^}]*)\}`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("main rule is missing")
	}
	for _, want := range []string{"padding:24px", "border-radius:12px", "border:1px solid var(--border)"} {
		if !strings.Contains(m[1], want) {
			t.Errorf("main rule lacks %q: %s", want, m[1])
		}
	}
	if strings.Contains(body, "shadow") {
		t.Error("card must not have a shadow")
	}
}

// tokensCSS は frontend/src/styles/tokens.css を読み、ライト（:root）とダーク（prefers-color-scheme: dark の :root）の
// トークン（--name: value）を返す。完了ページの値の複製がトークンとずれていないかを検証するための読み取り。
func tokensCSS(t *testing.T) (light, dark map[string]string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "styles", "tokens.css"))
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	const darkStart, darkEnd = "@media (prefers-color-scheme: dark)", "@media (prefers-reduced-motion"
	i, j := strings.Index(css, darkStart), strings.Index(css, darkEnd)
	if i < 0 || j < i {
		t.Fatal("dark block is not found in tokens.css")
	}
	decl := regexp.MustCompile(`(?m)^\s*(--[a-z0-9-]+):\s*([^;]+);`)
	parse := func(s string) map[string]string {
		m := map[string]string{}
		for _, d := range decl.FindAllStringSubmatch(s, -1) {
			m[d[1]] = strings.TrimSpace(d[2])
		}
		return m
	}
	return parse(css[:i]), parse(css[i:j])
}

// 完了ページが複製している値（色・フォント・サイズ・行間・角丸・余白）は tokens.css と一致する。
// トークンを変えたらこのテストが落ちるので、callbackpage.go の page も合わせて直す（ビルド時の共有はしない。docs/components/overview.md §5.3）。
func TestPageValuesMatchTokens(t *testing.T) {
	_, body := render(t, "x")
	light, dark := tokensCSS(t)

	root := regexp.MustCompile(`:root\{([^}]*)\}`).FindAllStringSubmatch(body, -1)
	if len(root) != 2 {
		t.Fatalf(":root rules = %d, want 2 (light, dark)", len(root))
	}
	for i, want := range []map[string]string{light, dark} {
		got := regexp.MustCompile(`(--[a-z]+):([^;]+)`).FindAllStringSubmatch(root[i][1], -1)
		if len(got) != 6 {
			t.Fatalf("scheme %d: %d tokens, want 6 (ink, bone, surface, border, mute, accent)", i, len(got))
		}
		for _, g := range got {
			if g[2] != want[g[1]] {
				t.Errorf("scheme %d: %s = %s, tokens.css has %s", i, g[1], g[2], want[g[1]])
			}
		}
	}

	// トークンが改名・削除されると下の期待値が空文字を含んで常に一致してしまうので、先に存在を確かめる。
	for _, k := range []string{"--font-sans", "--font-size-lg", "--line-height-lg", "--font-size-base", "--line-height-base", "--s-6", "--r-lg", "--s-3", "--mute", "--ink", "--accent", "--r-md", "--s-4"} {
		if light[k] == "" {
			t.Fatalf("tokens.css has no %s", k)
		}
	}

	// フォントはトークンの family（"Sora Variable" 等は同梱フォントの @font-face 名）から " Variable" を外した、端末のフォント名。
	fam := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(light["--font-sans"], " Variable", ""), `"Sora"`, "Sora"), ", ", ",")
	if want := "font-family:" + fam; !strings.Contains(body, want) {
		t.Errorf("font-family does not match --font-sans: want %q", want)
	}
	for _, want := range []string{
		"font-size:" + light["--font-size-lg"] + ";line-height:" + light["--line-height-lg"],
		"font-size:" + light["--font-size-base"] + ";line-height:" + light["--line-height-base"],
		"padding:" + light["--s-6"],
		"border-radius:" + light["--r-lg"],
		"margin:0 0 " + light["--s-3"],
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q (tokens.css と不一致)", want)
		}
	}
}

// viewport のスクロールバーはガイド §10 の設計（8px・矢印なし・--mute の thumb・hover/active は --ink 35%/65%）で、
// frontend/src/styles/scrollbar.css と同じ構造・値にする（B1・B2）。値が scrollbar.css とずれたら落ちる。
func TestPageScrollbarMatchesScrollbarCSS(t *testing.T) {
	_, body := render(t, "x")
	b, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "styles", "scrollbar.css"))
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)
	size := regexp.MustCompile(`::-webkit-scrollbar \{\s*width: (\d+px);\s*height: (\d+px);`).FindStringSubmatch(css)
	hover := regexp.MustCompile(`thumb:hover[^{]*\{\s*background: color-mix\(in srgb, var\(--ink\) (\d+)%, var\(--mute\)\)`).FindStringSubmatch(css)
	active := regexp.MustCompile(`thumb:active[^{]*\{\s*background: color-mix\(in srgb, var\(--ink\) (\d+)%, var\(--mute\)\)`).FindStringSubmatch(css)
	if size == nil || hover == nil || active == nil {
		t.Fatal("scrollbar.css の寸法・hover・active の宣言が読めない")
	}
	for _, want := range []string{
		"@media (forced-colors:none){",
		"@supports selector(::-webkit-scrollbar){",
		"scrollbar-color:var(--mute) transparent",
		":root,:root *{scrollbar-width:auto;scrollbar-color:auto}",
		"::-webkit-scrollbar{width:" + size[1] + ";height:" + size[2] + "}",
		"::-webkit-scrollbar-track,::-webkit-scrollbar-corner{background:transparent}",
		"::-webkit-scrollbar-button{display:none}",
		"::-webkit-scrollbar-thumb{background:var(--mute);border-radius:999px}",
		"color-mix(in srgb,var(--ink) " + hover[1] + "%,var(--mute))",
		"color-mix(in srgb,var(--ink) " + active[1] + "%,var(--mute))",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q (scrollbar.css と不一致)", want)
		}
	}
	if strings.Contains(body, "%!") {
		t.Errorf("page has a broken format verb: %s", body)
	}
}

// Android 以外（Windows など）の完了ページは、アプリへ戻るリンクも自動遷移も持たない（Windows の動きを変えない）。
func TestWriteHasNoReturnAffordanceOutsideAndroid(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skip("android では戻るリンクを付ける（TestWriteAddsReturnLinkOnAndroid が検証）")
	}
	_, body := render(t, "x")
	for _, bad := range []string{AndroidReturnURL, "http-equiv", `class="back"`, "戻る</a>"} {
		if strings.Contains(body, bad) {
			t.Errorf("non-android page contains %q: %s", bad, body)
		}
	}
}

// Android の完了ページは「Nefos Inbox に戻る」リンク（主）と、同じ行き先への meta refresh（補助）を持つ。
// 戻る URL は認可コード・state・トークンを含まない固定の値で、CSP はスクリプトを許さないままにする。
func TestWriteAddsReturnLinkOnAndroid(t *testing.T) {
	rec := httptest.NewRecorder()
	write(rec, "認可の操作が完了しました。", AndroidReturnURL)
	body := rec.Body.String()
	for _, want := range []string{
		`<a class="back" href="` + AndroidReturnURL + `">Nefos Inbox に戻る</a>`,
		`<meta http-equiv="refresh" content="0;url=` + AndroidReturnURL + `">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %q: %s", want, body)
		}
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "default-src 'none'; style-src 'unsafe-inline'" {
		t.Errorf("CSP = %q（スクリプトを許可しない）", got)
	}
	if strings.Contains(body, "<script") {
		t.Error("page must not contain a script")
	}
	if strings.Contains(body, "%!") {
		t.Errorf("page has a broken format verb: %s", body)
	}
	// 戻る URL に認可の秘密が入らない: scheme://host だけで、クエリ・パス・フラグメントを持たない。
	u, err := url.Parse(AndroidReturnURL)
	if err != nil {
		t.Fatal(err)
	}
	if u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.User != nil {
		t.Errorf("return URL carries more than scheme and host: %s", AndroidReturnURL)
	}
}

// 戻るリンクの URL は属性値としてエスケープされる（呼び出し元が任意の文字列を渡しても属性を壊さない）。
func TestWriteEscapesReturnURL(t *testing.T) {
	rec := httptest.NewRecorder()
	write(rec, "x", `a"><script>alert(1)</script>`)
	if body := rec.Body.String(); strings.Contains(body, "<script>") {
		t.Errorf("return URL is not escaped: %s", body)
	}
}

// 戻るリンクのスタイルは色をトークン（:root の変数）経由だけにし、タップしやすい高さ（44px 以上）を持つ（デザインガイド C2・A11y）。
func TestReturnLinkStyle(t *testing.T) {
	rec := httptest.NewRecorder()
	write(rec, "x", AndroidReturnURL)
	m := regexp.MustCompile(`a\.back\{([^}]*)\}`).FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatal("a.back rule is missing")
	}
	for _, want := range []string{"background:var(--accent)", "color:var(--bone)", "border-radius:8px", "padding:12px 16px", "font-size:16px;line-height:1.8"} {
		if !strings.Contains(m[1], want) {
			t.Errorf("a.back lacks %q: %s", want, m[1])
		}
	}
	// 12px*2 + 16px*1.8 = 52.8px ≥ 44px
}

// 戻るリンクは、ガイドの primary ボタン（--accent の地・--bone の文字・--r-md・--s-4 の左右の余白）と同じ値（デザインガイド C2・S 系）。
func TestReturnLinkMatchesPrimaryButtonTokens(t *testing.T) {
	light, _ := tokensCSS(t)
	rec := httptest.NewRecorder()
	write(rec, "x", AndroidReturnURL)
	m := regexp.MustCompile(`a\.back\{([^}]*)\}`).FindStringSubmatch(rec.Body.String())
	if m == nil {
		t.Fatal("a.back rule is missing")
	}
	for _, want := range []string{"border-radius:" + light["--r-md"], "padding:12px " + light["--s-4"]} {
		if !strings.Contains(m[1], want) {
			t.Errorf("a.back lacks %q (tokens.css と不一致): %s", want, m[1])
		}
	}
}

// AndroidReturnURL は、MainActivity の BROWSABLE の intent-filter（scheme・host）と同じ値。ずれると、完了ページのリンクを押してもアプリが開かない。
func TestAndroidReturnURLMatchesManifest(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "build", "android", "app", "src", "main", "AndroidManifest.xml"))
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(AndroidReturnURL)
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(b)
	var filter string
	for _, m := range regexp.MustCompile(`(?s)<intent-filter>(.*?)</intent-filter>`).FindAllStringSubmatch(manifest, -1) {
		if strings.Contains(m[1], "android.intent.category.BROWSABLE") {
			filter = m[1]
		}
	}
	if filter == "" {
		t.Fatal("AndroidManifest.xml に BROWSABLE の intent-filter が無い")
	}
	for _, want := range []string{
		`android.intent.action.VIEW`,
		`android.intent.category.DEFAULT`,
		`android:scheme="` + u.Scheme + `"`,
		`android:host="` + u.Host + `"`,
	} {
		if !strings.Contains(filter, want) {
			t.Errorf("BROWSABLE の intent-filter に %q が無い: %s", want, filter)
		}
	}
	if n := strings.Count(manifest, "android.intent.category.BROWSABLE"); n != 1 {
		t.Errorf("BROWSABLE の intent-filter は 1 つだけ（増やすときは docs/environment/android.md §14.2 と verify-apk.sh を直す）: %d 個", n)
	}
}

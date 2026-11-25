---
title: Sık sorulan sorular
sidebar:
  order: 14
---

Yaygın hatalar, beklenmedik durumlar ve davranış soruları. Sorununuz burada yoksa çalıştırdığınız adımlar ve tam çıktıyla [GitHub kaydı](https://github.com/alibaba/open-code-review/issues) açın.

## Yapılandırma ve başlangıç {#configuration-startup}

### `no valid LLM endpoint configured` {#no-valid-llm-endpoint-configured}

```
no valid LLM endpoint configured; one of OCR_LLM_URL/OCR_LLM_TOKEN/OCR_LLM_MODEL,
~/.opencodereview/config.json, or ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/
ANTHROPIC_MODEL must be set
```

OCR tüm uç nokta çözümleme zincirini ([Yapılandırma](../configuration/#reuse-existing-environment-variables)) denedi ancak eksiksiz `(URL, token, model)` üçlüsü bulamadı. Şunlardan birini yapın:

- `ocr config set llm.url …` / `llm.auth_token …` / `llm.model …` ile `~/.opencodereview/config.json` dosyasını doldurun **veya**
- `OCR_LLM_URL` / `OCR_LLM_TOKEN` / `OCR_LLM_MODEL` değişkenlerini dışa aktarın **veya**
- Claude Code kullanıyorsanız `ANTHROPIC_BASE_URL` / `ANTHROPIC_AUTH_TOKEN` / `ANTHROPIC_MODEL` değişkenlerini dışa aktarın.

İncelemeyi yeniden denemeden önce `ocr llm test` ile bağlantıyı doğrulayın.

### `ocr llm test` yanlış kaynağı gösteriyor {#ocr-llm-test-shows-the-wrong-source}

OCR son değil **ilk** eksiksiz üçlüyü kullanır. Yapılandırma dosyasında üç llm.* anahtarı da varsa ortam değişkenleri yok sayılır. Ortamın öncelikli olması için yapılandırma anahtarlarını kaldırın (dosyayı silin veya alanları elle kaldırın) ya da `ocr config set` ile yeni değerlere geçin.

### `ocr llm test` 401 / 403 veriyor {#401-403-from-ocr-llm-test}

Token'ın kapsamı eksik, süresi dolmuş veya başka sağlayıcıya ait olabilir. Anthropic ve OpenAI farklı kimlik doğrulama başlıkları ve URL biçimleri kullanır; `llm.use_anthropic` değerinin URL'yle uyumunu kontrol edin:

- Anthropic: URL `/v1/messages` ile biter, `use_anthropic=true`.
- OpenAI / uyumlu sağlayıcı: URL `/v1/chat/completions` ile biter, `use_anthropic=false`.

### `not a git repository` {#not-a-git-repository}

`ocr review`, geçerli dizinde `git diff` ve izlenmeyen dosyalar için `git ls-files` çalıştırır. Git çalışma ağacı içinde değilseniz erken çıkar. `cd` ile depoya geçin veya `--repo /path/to/repo` belirtin.

### “No tool calls parsed” (yerel modeller / Ollama) {#no-tool-calls-parsed-local-models-ollama}

```
[ocr] No tool calls parsed for src/foo.go, retrying...
[ocr] Max tool requests reached for src/foo.go.
```

Her inceleme `No tool calls parsed` tekrarlarıyla dönüp “Max tool requests reached” ve sıfır yorumla bitiyorsa sorun yapılandırma değil modeldir. OCR incelemeyi tamamen araç çağrılarıyla yürütür; **modelin yerel araç çağırma (function calling) desteği olmalıdır**. Araç çağrılarını yalnızca metin çıktısında veya `<think>` bloklarında *anlatan* model, istem nasıl ayarlanırsa ayarlansın çalışmaz; `deepseek-r1` yaygın bir örnektir. `qwen3` gibi yerel araç desteği bulunan modeller çalışır. Ollama'da araç destekli modellerden seçin: <https://ollama.com/search?c=tools>.

Yerel modeli OCR olmadan doğrudan doğrulayın:

```bash
curl http://127.0.0.1:11434/v1/chat/completions -H "Content-Type: application/json" -d '{
  "model": "qwen3:32b",
  "messages": [{"role": "user", "content": "The code below has a bug, use the report_bug tool to report it.\n\nfunc add(a, b int) int {\n  return a - b\n}"}],
  "tools": [{"type": "function", "function": {"name": "report_bug", "description": "Report a bug in the code",
    "parameters": {"type": "object", "properties": {"line": {"type": "integer"}, "description": {"type": "string"}}, "required": ["description"]}}}]
}'
```

Başarılı: yanıtta `report_bug` adlı yapılandırılmış `tool_calls` dizisi vardır. Başarısız: “çağrı”, `content` içinde düz metindir.

Model araçları destekliyor ancak yerel donanımda yavaş yanıtlıyorsa LLM zaman aşımını artırın; [Zaman aşımları](../configuration/#timeouts) bölümüne bakın.

## Filtreleme ve kurallar {#filtering-rules}

### Dosyam incelenmiyor {#my-file-isn-t-being-reviewed}

`ocr review --preview` çalıştırın (LLM maliyeti yoktur). Her aday dosyanın tutulma veya elenme **nedeni** gösterilir. `vendor/` ve `node_modules/` gibi bağımlılık dizinleri terminalde tek özet satırında toplanır; `ocr review --preview --format json` her girdiyi ayrı listeler:

```
src/foo.go              modified
src/foo_test.go         modified  (excluded: user_exclude)
imgs/logo.png           binary    (excluded: unsupported_ext)
3 file(s) in provider directories (node_modules/) — not reviewable
```

Dışlama nedenleri [dosya filtresindeki](../review-rules/#how-files-are-filtered) aşamalara karşılık gelir:

| Neden | Çözüm |
|---|---|
| `binary` | İşlem gerekmez; ikili dosyada incelenebilir metin yoktur. |
| `secret_exclude` | İşlem gerekmez; `.env`, `id_rsa` ve `.netrc` gibi kimlik bilgisi dosyaları bir `include` kalıbıyla eşleşse bile incelemeye alınmaz. |
| `user_exclude` | Deseni `exclude` listesinden kaldırın. |
| `unsupported_ext` | İzin listesi kontrolünü aşmak için uzantıyı `include` listesine ekleyin. |
| `default_path` | Yerleşik test dosyası dışlamasını aşmak için dosyayı `include` listesine ekleyin. |
| `provider_directory` | İşlem gerekmez; `vendor/`, `node_modules/` gibi dizinler include ile bile incelenmez. |
| `deleted` | İncelenecek yeni içerik yoktur. |
| `too_large` | Fark tek başına `max_tokens` değerinin %80'ini aşar. `--max-tokens` veya kayıtlı `max_tokens` değerini artırın ya da değişikliği küçük commit'lere bölün. |

### Özel kuralım uygulanmıyor {#my-custom-rule-isn-t-firing}

`ocr rules check <file-path>` çalıştırın. Eşleşen **katmanı** ve **glob desenini** gösterir:

```
File: src/api/UserHandler.go
Source: Project (.opencodereview/rule.json)
Pattern: src/api/**/*.go
Rule: …
```

Katman yanlışsa (örneğin proje kuralı beklerken “System built-in”) büyük olasılıkla **tanım sırası** etkilidir: ilk eşleşme kazanır. Daha özel kuralı `rules` dizisinin başına taşıyın veya glob desenini düzeltin.

### Süslü parantez genişletmesi çalışmıyor {#brace-expansion-isn-t-working}

`bmatcuk/doublestar/v4`, `{ts,tsx}` biçimini destekler. Eşleşmiyorsa gereksiz boşlukları kontrol edin; `{ts, tsx}` içindeki boşluk `tsx` eşleşmesini sessizce engeller.

## İncelemeler {#reviews}

### Dosyada sıfır yorum var; gerçekten incelendi mi? {#a-file-shows-zero-comments-was-it-actually-reviewed}

[Oturum görüntüleyicisi](../viewer/)ni (`ocr viewer`) açıp oturumu bulun. Dosyanın bulunduğu grubun `main_task` şeridine bakın. Grupların anahtarı dosya yollarıdır; tek başına incelenen dosya kendi yoluyla görünür:

- Araç çağrıları var ve `task_done` ile bitiyor → inceleme sorunsuz tamamlandı.
- Araç çağrıları var ve döngü ortasında bitiyor → hata kartını arayın.
- Hiç `main_task` kartı yok → inceleme öncesinde elendi; [Filtreleme ve kurallar](#filtering-rules) bölümüne bakın.

### Yorumlarda `start_line: 0` ve `end_line: 0` var {#comments-have-startline-0-and-endline-0}

OCR yorumu farktaki kesin satıra bağlayamadı. Yaygın iki neden:

- Model `existing_code` alanını farktan aynen kopyalamak yerine yeniden yazdı. Aksi söylense de bazen bunu yapar.
- Farkın alışılmadık biçimi (CRLF, karışık sekme/boşluk) kayan pencere eşleşmesini bozdu.

Yorum yine geçerlidir; yalnızca otomatik konumlandırılamamıştır. Çoğu ajan entegrasyonu (SKILL, Claude Code eklentisi) `existing_code` alanını okuyup dosyadaki yeri kendisi bulur.

### Token eşiği aşıldı {#token-threshold-exceeded}

```
[ocr] WARNING: prompt tokens (240000) exceed 80% of max_tokens(200000) [round 1] for group "src/big.sql"
```

Grubun ilk istemi (kural + farklar + değişen dosya listesi), model yanıt vermeden `MAX_TOKENS = 200000` değerinin %80'ini aşmıştır. OCR grubu atlayıp devam eder; JSON modunda `warnings` içinde de görünür.

`MAX_TOKENS` yalnızca **istem** tavanıdır. Model çıktısı ayrı `MAX_COMPLETION_TOKENS` (`16384`) değeriyle sınırlanır; bu uyarı her zaman girdi boyutuyla ilgilidir.

Çözümler:

- Otomatik üretilen dosyayı `exclude` listesine ekleyin.
- Büyük yeniden düzenlemeyi küçük commit'lere bölün.
- Küçük commit serisini çalışma alanında topluca incelemek yerine `--commit` modunu kullanın.

### Dosya küçük ama plan aşaması çok uzun sürdü {#plan-phase-took-forever-and-the-file-is-small}

Önce `ocr review --preview` çalıştırın. Planlama iki eşikten **biri** aşılınca çalışır:

- Gruptaki en büyük dosyada en az `PLAN_MODE_LINE_THRESHOLD` satır değişmişse (varsayılan **50**) **veya**
- Grup 2+ dosya içeriyor ve toplam `lines.changed`, `PLAN_MODE_GROUP_LINE_THRESHOLD` değerine ulaşıyorsa (varsayılan **100**).

Küçük dosya, toplamı büyük olan diğer değişikliklerle gruplanmışsa planlamaya girer. Bu tasarım gereğidir; geniş veya büyük farklar planlamadan yararlanır. Küçük fark her iki eşiğin altında kalabilir. Planlama, araç geçersiz kılmalarıyla değil gömülü şablonla yönetilir; temel davranış değişikliği kaynak değişikliği ve yeniden derleme gerektirir.

### “Max tool requests reached” {#max-tool-requests-reached}

```
[ocr] Max tool requests reached for src/foo.go.
```

Model `task_done` çağırmadan 100 (`MAX_TOOL_REQUEST_TIMES`) araç kullanım turunu tüketmiştir. O ana kadar üretilen yorumlar yine toplanıp gösterilir. Çoğu grupta oluyorsa yaygın nedenler:

- Model “bitince `task_done` çağır” yönergesini iyi izlemiyordur; daha güçlü model kullanın (ör. Claude Opus).
- Araç sürekli hata verir, model yeniden dener. Oturum JSONL kaydında tekrarlanan aynı araç sonucunu kontrol edin.
- Grup gerçekten büyük veya yoğun bağlamlıdır; 100 tur yetmez. `--max-tools <n>` ile artırın (ör. `150`). Çözümlenen pozitif değer sınırı tam belirler; 1–49 değerleri `50`'ye yükseltilir, `0` şablon varsayılanı `100`'ü korur.
- Model yerel araç çağırmayı desteklemiyordur; [“No tool calls parsed”](#no-tool-calls-parsed-local-models-ollama) bölümüne bakın.

### Bazı alt ajanlar başarısız; çıkış kodu neden sıfır değil? {#some-sub-agents-fail-why-is-the-exit-code-non-zero}

OCR grup hatalarını yalıtır ve tamamlanan grupların bulgularını yayımlar. Ancak zaman aşımı, sağlayıcı hatası veya bozuk yapılandırma gibi işlemsel hatalar, diğer gruplar başarılı olsa bile CI'nin eksik kapsamı algılaması için sıfırdan farklı çıkış üretir. Yalnızca yapılandırılmış bütçe sınırından kaynaklanan kısmi inceleme başarılı çıkış sayılır. Başarısız grupları ve devam edilebilirliği JSON'da `manifest.coverage.failed`, `warnings` veya metin özetinden kontrol edin.

### CI çalıştırması yerelden çok yavaş {#ci-run-is-much-slower-than-local}

- **Model hız sınırları** — LLM istemcisi sınırlamada bekleyip yeniden dener. Sınıra takılmamak için `--concurrency` değerini düşürün (ör. `4`).
- **Soğuk önbellek** — sağlayıcı istem önbelleğini destekliyorsa dağıtım sonrası ilk çalıştırma bundan yararlanmaz. Aynı pencere içindeki sonrakiler daha hızlıdır.

## Çıktı ve entegrasyon {#output-integration}

### `--audience agent` hâlâ ilerleme satırları gösteriyor {#audience-agent-still-has-progress-lines}

**stderr** görmediğinizden emin olun. Uyarı ve hatalar zaman zaman stderr'e gider. `--audience agent` tarafından sağlanan temiz stdout ayrıştırmaya uygundur. Her şeyi gizlemek için `ocr review --audience agent 2>/dev/null` kullanın.

### JSON çıktısı `{ "files_reviewed": 0, "comments": [] }` {#json-output-is-filesreviewed-0-comments}

İncelemeye uygun dosya yoktur. `files_reviewed` üst düzey alan değildir; `summary` altında bu durumda `0` olur, `comments` ise üst düzey `[]` değeridir. Aynı nesne `"status": "skipped"`, `"message": "Review skipped: no items were selected."` ve `terminal_state` değeri `"skipped"`, `coverage.selected` değeri boş olan `manifest` taşır. Seçimde elenen dosyalar, `exclude_reason` nedenleriyle `coverage.excluded` içinde listelenir.

Dosyaları inceleyip sorun bulmayan çalıştırma da **`comments: []` içeren JSON nesnesi** döndürür. `summary.files_reviewed` gerçekten incelenen sayıyı, `status` değeri `"complete"` durumunu, `message` ise `"Review complete: 0 finding(s) across N selected item(s)."` metnini gösterir. İsteğe bağlı üst düzey alanlar değişebilir; durumları nesne biçiminden veya isteğe bağlı anahtar varlığından ayırmayın. Manifest içeren inceleme çıktısında `summary.files_reviewed` veya `manifest.terminal_state` kullanın. Aşağıdaki manifestsiz yolda `summary` ve `manifest` bulunmayabileceğini de hesaba katın. `review --format json`, stdout'a her zaman tek JSON nesnesi yazar; yalın dizi yazmaz.

`terminal_state: "complete"`, tüm değişen dosyaların değil **seçili kümenin** tamamlandığını belirtir. Seçimde elenen dosyalar (desteklenmeyen uzantı, yol kuralları, yerleşik gizli bilgi yolları, ikili, silinmiş veya boyut sınırını aşan dosyalar), `--preview` ile aynı `exclude_reason` kullanılarak `manifest.coverage.excluded` içinde kaydedilir; çalıştırmayı `partial` yapmaz. Liste yalnızca seçim aşamasına ulaşan dosyaları kapsar. Fark sağlayıcısının daha önce attıkları burada yer almaz: `vendor/`, `node_modules/` gibi her zaman atlanan dizinler; `.gitignore` tarafından yok sayılan izlenmeyen dosyalar; commit veya aralık incelemesinde `.gitignore` ile eşleşen tüm değişiklikler.

Manifestsiz dosya-yok yolu daha sadedir: `summary` ve `manifest` yoktur; `"status": "skipped"`, `"message": "No supported files changed."`, `"comments": []` ve her zaman `tool_calls` bulunur. `ocr scan` her zaman manifestsizdir; dosya-yok kontrolü eşleşirse bu yolu kullanır. `ocr review` da manifest oluşturulamaz ve dosya-yok kontrolü eşleşirse aynı yola girebilir. `llm`, `trace_id` gibi isteğe bağlı meta veriler bulunabilir.

### Oturum JSONL dosyaları nerede? {#where-do-session-jsonls-live}

```
~/.opencodereview/sessions/<path-encoded-repo-path>/<session-id>.jsonl
```

Depo yolu `/` ve `\` yerine `-`, `:` yerine `_` konularak kodlanır (ör. `/Users/foo/my-repo` → `Users-foo-my-repo`). `ocr viewer` ile göz atın. Geçmişi silmek için dizini kaldırın; OCR sonraki çalıştırmada kodlanmış yolu yeniden oluşturur.

## Performans ve maliyet {#performance-cost}

### Token kullanımını nasıl görebilirim? {#how-can-i-tell-what-tokens-cost-what}

Telemetriyi açın:

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter console
ocr review
```

Ana döngüde LLM çağrıları `llm.request` span'leri ve metrikler üretir. `ocr.llm.tokens_used` (`model` + `type` etiketli sayaç), `ocr.llm.requests_total` (`model` + `status` etiketli sayaç), `ocr.llm.request_duration_seconds` (`model` etiketli histogram) değerlerini izleyin. Console dışa aktarıcısı toplamları satır içinde gösterir. Panolar için OTLP'ye geçip metrik sisteminize gönderin; [Telemetri](../telemetry/) sayfasına bakın.

### İncelemeler neden pahalı? {#why-are-my-reviews-so-expensive}

- Çaba ayarı grup başına tur sayısını belirler: `low` = 1, `medium` (varsayılan) = 2, `high` = 3. Maliyet yaklaşık tur sayısıyla ölçeklenir; ucuz çalıştırma için en büyük etki `--effort low`, en pahalı seçenek `--effort high` olur.
- En büyük dosyası ≥ 50 değişen satır olan veya 2+ dosyası toplam ≥ 100 satır değişen gruplar planlanır. Grup başına ek LLM çağrısı gerektiğinden eşikleri **yükseltmek** maliyeti düşürür; düşürmek artırır. Sıfırda davranışları farklıdır: `PLAN_MODE_LINE_THRESHOLD` ≤ 0, *her zaman planla* demektir ve en pahalı ayardır; `PLAN_MODE_GROUP_LINE_THRESHOLD` = 0 grup eşiğini kapatır. Grup eşiği tek tetikleyiciyse plan çağrısı tasarruf edilir. Tetikleme kuralları için yukarıdaki plan aşaması sorusuna bakın.
- `MAX_TOOL_REQUEST_TIMES = 100` geniş bir bütçedir. Tüm turları kullanan model, 3 turda bitirenden daha uzun konuşma ve fazla token üretir. Güçlü modeller genellikle daha hızlı bitirir. Sınır hatasını aşmak için `--max-tools` artırırsanız grup maliyetinin yaklaşık doğrusal artmasını bekleyin.
- Bellek sıkıştırma da LLM çağrısıdır; uzun alt görevler inceleme yanında sıkıştırma turlarının maliyetini öder.
- Anlamsal gruplama çalıştırma başına küçük bir LLM çağrısı ekler. Yalnızca dosya meta verilerini (yol, durum, ekleme/silme sayısı), asla fark içeriğini görür. Ucuzdur ve ilişkili dosyaları birlikte inceleyerek genellikle maliyetini karşılar.

### LLM çağrılarını nasıl azaltırım? {#how-do-i-reduce-llm-calls}

- İlgilenmediğiniz dosyaları incelememek için `include` listesi ekleyin.
- Hesabınızda ani yoğunluğa göre fiyatlandırma varsa `--concurrency` düşürün.
- `--background` kullanın; başlangıçtaki iyi bağlam bazen `file_read` / `code_search` çağrılarını azaltır.

## Gizlilik ve güvenlik {#privacy-security}

### OCR kodumu bir yere gönderiyor mu? {#does-ocr-send-my-code-anywhere}

OCR **farklarınızı** ve isteğe bağlı okuma aracı parçalarını yapılandırdığınız LLM uç noktasına gönderir. Bunun dışında oturum JSONL ve kural dosyaları yereldir.

Telemetride `content_logging` yapılandırma katmanından geçer ancak şu anda **hiçbir** kod yolunu kontrol etmez; değerden bağımsız olarak istem ve yanıt içeriği toplayıcıya aktarılmaz. Ayrılmış seçenek olarak değerlendirin; üretimde `false` bırakın. [Telemetri](../telemetry/#content-logging) bölümüne bakın.

### LLM'ye gitmeden gizli bilgileri maskeleyebilir miyim? {#can-i-redact-secrets-before-they-re-sent-to-the-llm}

Yerleşik destek yoktur. Önerilen yöntem:

1. Gizli bilgileri depoya commit etmeyin.
2. Hash verisi içerdiği bilinen dosyaları `exclude` listesine ekleyin.
3. Gizli bilgileri farklardan uzak tutmak için `git diff --no-textconv` filtreleri veya commit öncesi maskeleme kullanın.

“Maskeleme kuralı” yol haritasındadır; [iş kayıtlarını](https://github.com/alibaba/open-code-review/issues) izleyin.

## Diğer {#misc}

### Değişiklik günlüğü nerede? {#where-s-the-changelog}

[GitHub Releases](https://github.com/alibaba/open-code-review/releases) sayfasında; her sürümün notları Conventional Commits üzerinden üretilir.

### Git dışındaki sürüm kontrol sistemleri destekleniyor mu? {#does-ocr-support-non-git-vcs}

Hayır. Fark sağlayıcıları `git` çalıştırır. SVN / Mercurial vb. yeni sağlayıcı gerektirir; Hg desteğiyle ilgili kayıt [buradadır](https://github.com/alibaba/open-code-review/issues).

### İkili dosya neden `opencodereview`, CLI ise `ocr`? {#why-is-the-binary-called-opencodereview-but-the-cli-is-ocr}

Sürümlerdeki statik ikili dosya projenin adını taşır (`opencodereview`); NPM sarmalayıcısı kolay kullanım için `ocr` olarak yükler. Kaynaktan derlemede `dist/opencodereview` (Windows'ta `dist/opencodereview.exe`) oluşur; `$PATH` üzerindeki bir yere `ocr` adıyla kopyalayın.

### Nasıl kaldırırım? {#how-do-i-uninstall}

```bash
npm uninstall -g @alibaba-group/open-code-review        # NPM install
sudo rm /usr/local/bin/ocr                              # binary install
rm -rf ~/.opencodereview                                # all state
```

OCR, NPM ikili dosya indirmesi dışında `~/.opencodereview` dışına yazmaz. Bu dizini kaldırmak geçmişi, yapılandırmayı ve kullanıcı kurallarını siler.

## Ayrıca bakın {#see-also}

- [Yapılandırma](../configuration/) — LLM uç noktası çözümleme ve anahtarlar.
- [İnceleme kuralları](../review-rules/) — dosya filtresi ve kural çözümleme zinciri.
- [Oturum görüntüleyicisi](../viewer/) — geçmiş incelemeleri inceleme.
- [Telemetri](../telemetry/) — token kullanımı ve LLM metrikleri.

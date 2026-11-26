---
title: Komut satırı başvurusu
sidebar:
  order: 6
---

Tüm `ocr` alt komutları, seçenekleri ve çıkış davranışları için kapsamlı başvuru.

## Genel kullanım {#global-usage}

```text
OpenCodeReview - AI-Powered Code Review CLI

Usage:
  ocr [command]

Commands:
  review, r    Start a code review
  rules        Inspect and debug review rules
  config       Manage configuration settings
  llm          LLM utility commands
  viewer       Start the WebUI session viewer
  session, sessions  List and inspect saved review sessions
  version      Show version information

Examples:
  ocr review --from master --to dev        Review diff range
  ocr review --commit abc123               Review a single commit
  ocr review --background "Focus on auth"                                           Review with inline context
  ocr review -B ./docs/requirements.md                                              Review with context file
  ocr config provider                      Interactive provider setup
  ocr config model                         Interactive model selection
  ocr config set llm.model opus-4-6        Set a config value
  ocr llm test                             Test LLM connectivity
  ocr llm providers                        List built-in providers
  ocr session list                         List saved review sessions
  ocr version                              Show version info

Use "ocr review -h" for more information about review.
Use "ocr rules -h" for more information about rules.
Use "ocr config" for more information about config.
Use "ocr llm" for more information about LLM utilities.
Use "ocr session -h" for more information about session inspection.

GitHub: https://github.com/alibaba/open-code-review
```

## Genel seçenekler {#global-flags}

Her komutta kullanılabilir; alt komuttan önce veya sonra verilebilir. `ocr --color=never review` ile `ocr review --color=never` eşdeğerdir.

| Seçenek | Varsayılan | İşlevi |
| --- | --- | --- |
| `--color <auto\|always\|never>` | `auto` | ANSI renklerinin ne zaman kullanılacağını belirler. `auto`, yalnızca stdout terminalse renklendirir; boru veya dosya yönlendirmesinde düz metin üretir. `always`, boruda da rengi korur (`\| less -R` için yararlıdır). |

Stdout terminal olmadığında metin çıktısı düz olduğundan güvenle başka komuta aktarılabilir:

```bash
ocr review --commit HEAD | gh issue comment 123 --body-file -
```

`TERM=dumb` da renkleri kapatır.

## Komut özeti {#command-summary}

| Komut | Takma ad | İşlevi |
| --- | --- | --- |
| `ocr review` | `ocr r` | Kod incelemesi yapar ve yorumları üretir. |
| `ocr scan` | `ocr s` | Git farkı gerektirmeden dosyaların tamamını tarar. |
| `ocr rules check <file>` | — | Yola uygulanan kuralı ve kaynağını gösterir. |
| `ocr config set <key> <value>` | — | Ayarı `~/.opencodereview/config.json` dosyasına kaydeder. |
| `ocr config unset custom_providers.<name>` | — | Özel sağlayıcıyı siler; etkinse `provider`/`model` değerlerini temizler. |
| `ocr config provider` | — | Etkileşimli sağlayıcı kurulumunu açar. |
| `ocr config model` | — | Etkileşimli model seçimini açar. |
| `ocr llm test` | — | Uç noktayı doğrulamak için küçük bir sohbet isteği gönderir. |
| `ocr llm providers` | — | Tüm yerleşik LLM sağlayıcılarını listeler. |
| `ocr session list` | `ocr sessions list`, `ocr session ls` | Kaydedilmiş inceleme oturumlarını listeler. |
| `ocr session show <id>` | `ocr sessions show <id>` | Oturumu ve dosya başına kontrol noktalarını gösterir. |
| `ocr session comments <id>` | `ocr sessions comments <id>` | Oturumdaki yorumları yazdırır. |
| `ocr session compare <before> <after>` | `ocr session diff <before> <after>` | Bulguları yeni, süren, giderilen ve incelenmeyen olarak karşılaştırır. |
| `ocr session export [id]` | — | Oturumu bağımsız HTML dosyasına aktarır. |
| `ocr session rm <id>` | `ocr session delete <id>`, `ocr session remove <id>` | Kayıtlı oturumu siler. |
| `ocr viewer` | — | Geçmiş oturumlar için yerel web arayüzünü açar (`localhost:5483`). |
| `ocr version` | — | Sürüm, commit, platform, derleme tarihi ve GitHub adresini yazdırır. |

`ocr` ve `ocr -h` genel kullanımı gösterir. Her alt komut `-h` / `--help` kabul eder.

## `ocr review` {#ocr-review}

Ana komuttur. Git farkını alır, değişen dosyaları anlamsal gruplara ayırır, grup başına alt ajan çalıştırır, yorumları toplar ve yazdırır.

`review` ve `scan`, son raporda bulguları metin, JSON ve SARIF biçimlerinde düşük önem derecesinden yükseğe doğru sıralar: `low`, `medium`, `high`, `critical`. Önem derecesi eksik veya tanınmayan bulgular `low` öncesinde gelir. Aynı derecedeki bulgular toplanma sırasını korur; paralel çalıştırmalarda bu sıra değişebilir.

### Kullanım biçimi {#synopsis}

```text
ocr review [flags]
ocr r      [flags]   (alias)
```

Seçenek verilmezse **çalışma alanı kipinde** çalışır; geçerli dizinin deposundaki indekse eklenmiş, eklenmemiş ve izlenmeyen tüm değişiklikleri inceler.

### Seçenekler {#flags}

| Seçenek | Kısa | Varsayılan | Açıklama |
| --- | --- | --- | --- |
| `--repo <path>` | — | Geçerli dizin | Git depo kökü. |
| `--from <ref>` | — | — | Farkın başlayacağı kaynak ref; ör. `main`. |
| `--to <ref>` | — | — | Farkın biteceği hedef ref; ör. `feature-branch`. `merge-base(from, to)..to` hesaplanır. `--from` verilip `--to` verilmezse `HEAD` kullanılır; yalnızca commit edilmiş değişiklikler, `git diff <from>...HEAD` gibi. |
| `--commit <sha>` | `-c` | — | Ebeveynine göre incelenecek tek commit. |
| `--preview` | `-p` | `false` | LLM'i çağırmadan filtreyi çalıştırır; dosyaları ve dışlama nedenlerini gösterir. `--format json` desteklenir, tamamlanmış bulgu bulunmadığından `sarif` desteklenmez. |
| `--no-filter` | — | `false` | Tüm yorumları korur, alt görev başına `REVIEW_FILTER_TASK` son işleme çağrısını atlar. Alt görev tek dosya veya ilişkili dosya grubudur. |
| `--resume <session-id>` | — | — | Uyumlu önceki aralık, commit veya çalışma alanı incelemesine devam eder. |
| `--format <fmt>` | `-f` | `text` | `text` (okunabilir metin), `json` (yapılandırılmış yorumlar) veya `sarif` (GitHub Code Scanning için SARIF 2.1.0). |
| `--output <path>` | `-o` | stdout | Sonuçları UTF-8 dosyaya yazar; `-`, stdout anlamına gelir. İlk yazmada oluşturulur; başarısız çalıştırma mevcut dosyaya dokunmaz. Metin biçiminde ANSI renkleri kaldırılır. |
| `--audience <who>` | — | `human` | `human`, ilerleme satırlarını gösterir; json/sarif biçiminde stdout ayrıştırılabilir kalsın diye stderr'e yazar. `agent`, ilerlemeyi tamamen gizler; yalnızca son özet/JSON'u yazdırır. |
| `--background <text>` | `-b` | — | Planlama ve ana isteme eklenen gereksinim/iş bağlamı. |
| `--background-file <path>` | `-B` | — | Bağlam olarak kullanılacak Markdown dosyası. İkisi de verilirse `--background` değerinden önceliklidir. |
| `--exclude <patterns>` | — | — | Virgülle ayrılmış gitignore biçiminde dışlama kalıpları; `rule.json` dışlamalarıyla birleştirilir. |
| `--concurrency <n>` | — | `8` | Aynı anda incelenen en fazla alt görev sayısı. |
| `--timeout <minutes>` | — | `15` | LLM, araçlar ve yeniden deneme beklemeleri dahil alt görev bütçesi. `0`, görev sınırını kaldırır; istek sınırını kaldırmaz. İstek sınırı bağımsızdır: saniye cinsinden `OCR_LLM_TIMEOUT` veya sağlayıcı `timeout_sec` (varsayılan `300`). Çaba turlarıyla doğrusal ölçeklenir; low/medium/high için 15/30/45 dakika. |
| `--effort <level>` | — | `medium` | `low`: 1, `medium`: 2, `high`: 3 tur. Fazla tur daha fazla bulgu ve orantılı maliyet sağlar. Bu çalıştırmada kayıtlı `effort` değerini geçersiz kılar. |
| `--rule <path>` | — | — | Proje ve genel `rule.json` yerine kullanılacak özel JSON kural dosyası. |
| `--max-tools <n>` | — | Şablon varsayılanı | Alt görev başına araç çağrısı turu sınırı. `0`, şablon varsayılanını (`100`) kullanır; 1–49 değerleri `50` yapılır. Çözümlenen pozitif değer kesin sınırdır. |
| `--max-tokens <n>` | — | Yapılandırma veya şablon | Hem inceleme hem analiz için alt görev başına istem/girdi token sınırı; şablon varsayılanı `200000`. Kayıtlı `max_tokens` değerinden önceliklidir. Çıktı sınırını değiştirmez; bkz. `MAX_COMPLETION_TOKENS`. |
| `--analysis` | — | Kapalı | Commit veya aralık incelemesiyle (`--commit` veya `--from`) birlikte ayrı bir davranış ve etki analizi çalıştırır. Yalnızca metin ve JSON çıktısını destekler. Çalışma alanında, önizlemede veya yarıda kalan incelemeye devam ederken kullanılmaz. Analiz, `--max-tools` değerinden bağımsız olarak 300 araştırma turu kullanabilir; bütçe tükenirse yalnızca sonucu göndermek için en fazla bir ek tur yapılır. Analizin başarısız olması veya atlanması incelemenin çıkış kodunu değiştirmez. |
| `--analysis-max-tokens-budget <n>` | — | `0` (sınırsız) | Yalnızca analizin toplam girdi ve çıktı token sayısına esnek bir sınır koyar. İncelemeyle eşzamanlı olarak bir ek model isteği çalışabilir. JSON çıktısında `analysis_meta.usage` ve `analysis_meta.execution_status` alanlarına bakın. |
| `--max-tokens-budget <n>` | — | `0` (sınırsız) | Toplam girdi+çıktı token sınırı. Her LLM turundan önce denetlenir. Bütçeyi aşan alt göreve bulguları göndermesi için son tur verilir ve `failed(budget)` bildirilir. Yeni görev dağıtılmaz; mevcut sonuçlar yazdırılır. |
| `--provider <name>` | — | — | Bu çalıştırmada kullanılacak yapılandırılmış sağlayıcı; `providers` ve `custom_providers` adları kabul edilir. |
| `--model <name>` | — | — | Çözümlenen modeli bu çalıştırma için değiştirir; ör. `claude-opus-4-6`. |
| `--max-git-procs <n>` | — | `16` | İnceleme ve analiz genelinde eşzamanlı çalışabilecek toplam Git alt süreci sayısı. |

> Kip seçenekleri birlikte kullanılamaz. Ya `--from`/`--to`, ya `--commit` verin ya da hiçbirini vermeyin (çalışma alanı). Karıştırmak hatadır. `--resume` üç kipte de çalışır ancak `--preview` ile birleşemez.

### Çalıştırmaya özel LLM seçimi {#per-run-llm-selection}

`review` ve `scan`, `--provider` ve `--model` kabul eder. Yalnızca mevcut çağrıyı etkiler; kayıtlı yapılandırmayı değiştirmez:

```bash
ocr review --provider anthropic --model claude-opus-4-6 --format json
ocr scan --provider openai --model gpt-5.4 --format json
```

Açık `--provider`, normal kaynak çözümlemesinden önce `providers` veya `custom_providers` girdisini seçer. Verilmezse eski kaynak sırası korunur: kayıtlı yapılandırma, eksiksiz `OCR_LLM_*` ortamı, eksiksiz Claude Code ortamı, ardından kabuk rc dosyaları. `--model`, kazanan kaynağın modelini değiştirir; sırayı değiştirmez. Eksik stratejiler birbiriyle karıştırılmadan atlanır. Seçili yerleşik sağlayıcının kimlik bilgileri desteklediği ortam değişkeninden yine alınabilir.

Yerleşik sağlayıcılarda `--model`, `ocr config model` önerilerinin dışındaki kimlikleri de kabul eder. Hem yerleşik listede hem `providers.<name>.models` içinde bulunmuyorsa stderr'e uyarı yazılır ve doğrulama sağlayıcıya bırakılır. Özel sağlayıcıların doğrulama kuralları değişmez.

### Kipler {#modes}

#### Çalışma alanı kipi (varsayılan) {#workspace-mode-default}

```bash
ocr review
```

OCR, çalışma ağacı değişikliklerini Git üzerinden toplar:

- İzlenen değişiklikler `git diff HEAD` ile; indekse eklenmiş ve eklenmemiş değişiklikler birlikte HEAD'e göre karşılaştırılır.
- İzlenmeyen dosyalar `git ls-files --others --exclude-standard` ile bulunur, diskten okunur ve tamamı yeni eklenmiş kabul edilir.

Bu kip commit öncesindeki çalışma ağacını incelemek içindir. İndekse eklenmemiş değişiklikler de kapsama girer. Karşılaştırmanın ayrıntıları için [Araçlar](../tools/#comparison-sides) belgesine bakın.

#### Aralık kipi {#range-mode}

```bash
ocr review --from main --to feature-branch
```

`merge-base(main, feature-branch)..feature-branch` hesaplanır. Böylece dal ayrıldıktan sonra `main` üzerine gelen ilgisiz değişiklikler yerine özellik dalının getirdiği fark görülür.

#### Commit kipi {#commit-mode}

```bash
ocr review --commit abc123
ocr review -c abc123
```

`git show abc123` çıktısını, yani o commit'in getirdiği değişiklikleri inceler.

Commit ve aralık incelemeleri, hedef commit'in (`--commit` veya `--to` ile belirlenen commit) kök dizinindeki `.gitignore` ile eşleşen değişiklikleri dışarıda bırakır; çalışma ağacındaki dosyayı kullanmaz. Analiz de değişiklik listesini aynı kurala göre oluşturur. Yok sayılan bir dosya yine de bağlam olarak okunabilir; analizin dosyayı okumasını engellemek için `--exclude` kullanın.

`--analysis` kullanıldığında `ocr viewer`, aynı oturum kimliğiyle **Review** ve **Analysis** sekmelerini tek bir raporda açar. **Analysis** sekmesinde yürütme durumu, kapsamdaki değişen dosyalar (okunan dosyaların listesi değil), modelin değerlendirmesi ve dayanakları gösterilir. Metindeki numaralı bir kaynağı seçerek ilgili kanıtı okuyabilir, **Evidence** bölümünü açarak atıf yapılan ve yapılmayan tüm gözlemleri görebilirsiniz. **Conversations** bölümünde kaydedilen analiz isteklerini, model yanıtlarını, araç çağrılarını ve sonuçlarını inceleyebilirsiniz. Bu kayıt HTML dışa aktarımına dahil edilmez. Analizin Markdown dosyası, sonuç üst verileri, kanıtları ve yürütme kaydı `~/.opencodereview/sessions/<encoded-repo>/<session-id>/analysis/` altında saklanır. Kanıtlar ve günlükler kaynak kodu içerebilir. Tamamlanmamış bir çalıştırmanın nihai durumu olmayabilir.

### Yarıda kalan incelemeye devam etme {#resuming-interrupted-reviews}

Her `ocr review`, `~/.opencodereview/sessions/` altında yerel günlük tutar. Başarılı metin çıktısı sonuçlara odaklanır, oturum kimliğini yazdırmaz. Kimliği `ocr session list/show` ile bulun veya `session_id` içeren `--format json` kullanın. Kesinti sonrasında aynı inceleme hedefiyle eşleşen oturuma devam edin:

```bash
ocr session list
ocr session show <session-id>
ocr session comments <session-id>
ocr review --from main --to feature-branch --resume <session-id>
ocr review --commit abc123 --resume <session-id>
ocr review --resume <session-id>   # workspace (no mode flags)
```

Devam ettirme bilinçli olarak katıdır. Kontrol noktaları yalnızca önceki çalıştırmayla aynı şey incelenecekse yeniden kullanılır:

- Çalışma alanında hem taban commit hem commit edilmemiş değişiklikler eşleşmelidir. Aralık/commit kipinden farklı olarak sabitlenecek bir commit olmadığından devam ederken yapılan düzenlemeler korunmaz. Kimlik, bayt bayt görüntüden değil seçilip ayrıştırılan farktan hesaplanır. İzlenmeyen dosyanın son satır sonunu eklemek/kaldırmak veya yalnızca LF/CRLF değiştirmek aynı farka normalleşir; devam edilebilir. İçerik değişikliği reddedilir.
- Kip eşleşmelidir; aralık oturumu commit oturumu olarak sürdürülemez.
- Çözümlenen girdi eşleşmelidir. Ref'lerin yazımı karşılaştırılmaz; `abc1234` ve `abc1234def` aynı commit olabilir. Aynı ref artık farklı farka çözümleniyorsa veya kural/filtreler seçilen dosyaları değiştirmişse kısmi kullanım yerine tüm devam isteği reddedilir.
- İncelemede kullanılan gereksinim bağlamı ve Plan ile Main istem şablonları
  (yanıt dili dahil) eşleşmelidir. Bağlam dosyaları temizlenmiş içerikleri
  üzerinden karşılaştırılır. Bunlardan biri değiştiyse veya önceki çalıştırmanın
  bildiriminde `review_prompt_sha256` yoksa `--resume` seçeneğini kaldırıp yeni bir
  inceleme başlatın. Önceki oturumu yine `ocr session show` ile görüntüleyebilirsiniz.
- Sağlayıcı/model değişikliği açıkça `--provider` / `--model` ile istenmelidir. Yapılandırma ya da ortamdan gelen değişiklik reddedilir.
- Önceki oturum, girdinin doğrulanacağı çalıştırma bildirimini içermelidir. Dosya dağıtımından sonra Ctrl-C incelemeyi düzgün iptal edip bildirim kaydeder; tamamlanan kontrol noktaları kullanılabilir kalır. Düzgün kapanmadan öldürülen süreçte ve bu özellikten eski oturumlarda bildirim yoktur.
- Yalnızca önceki bildirimin tamamladığını belirttiği dosyalar kullanılır. Bildirimde olmayan veya okunamayan kontrol noktası yalnızca ilgili dosyayı etkiler; o dosya yeniden incelenir.
- `--preview` ile `--resume` birlikte kullanılamaz.

Reddedilen devam isteği hiçbir şey yazmaz: oturum, bildirim veya LLM çağrısı oluşturulmaz.

### Çıktı {#output}

#### Metin (varsayılan, `--audience human`) {#text-default-audience-human}

İlerleme satırları canlı gösterilir. Sonrasında yorum başına bir blok gelir: soluk Unicode çizgili `path:start-end` başlığı, 100 sütuna sarılan yorum metni ve varsa önerilen değişikliğin renkli satır içi farkı. Sonunda stdout'a özet yazılır:

```
[ocr] 17 file(s) changed, reviewing 9 in /path/to/repo
[ocr] Skipping image.png — filtered by path/extension rules
[ocr]   ▶ file_read "src/foo.go"
[ocr]   ✔ file_read (12ms)
[ocr] Plan completed for src/foo.go
…

─── src/foo.go:42-47 ───
Concurrent map access without a lock — wrap with sync.RWMutex.

- m[k] = v
+ mu.Lock(); defer mu.Unlock(); m[k] = v

…
[ocr] Summary: 9 file(s) reviewed, 14 comment(s), ~21344 token(s) used (input: ~18012, output: ~3332), 1m12s elapsed
```

#### Metin (ajan, `--audience agent`) {#text-agent-audience-agent}

Yorum çıktısı aynıdır; ilerleme satırları sessizleştirilebilir stdout yazıcısı ([`internal/stdout`](https://github.com/alibaba/open-code-review/blob/main/internal/stdout/stdout.go)) ile bastırılır. CI'da veya başka ajana aktarırken kullanın.

#### JSON {#json}

```bash
ocr review --format json --audience agent
```

Belge stdout'a tek başına yazılır. Varsayılan `--audience human` ile `[ocr]` ilerleme satırları **stderr** üzerinden gelir. Böylece uzun çalıştırmayı izlerken stdout'u doğrudan ayrıştırıcıya gönderebilirsiniz:

```bash
ocr review --format json > result.json   # progress still visible on the terminal
ocr review --format json | jq .summary   # stdout is a single JSON document
```

İlerlemeyi tamamen kaldırmak için `--audience agent`, kabukta atmak için `2>/dev/null` kullanın.

```json
{
  "status": "success",
  "llm": {
    "provider": "anthropic",
    "model": "claude-opus-4-6"
  },
  "summary": {
    "files_reviewed": 9,
    "comments": 1,
    "total_tokens": 21344,
    "input_tokens": 18012,
    "output_tokens": 3332,
    "elapsed": "1m12s"
  },
  "comments": [
    {
      "path": "src/foo.go",
      "content": "Concurrent map access without a lock — wrap with sync.RWMutex.",
      "start_line": 42,
      "end_line": 47,
      "existing_code": "m[k] = v",
      "suggestion_code": "mu.Lock(); defer mu.Unlock(); m[k] = v",
      "thinking": "Looking at line 42, the map …"
    }
  ]
}
```

Üst düzey alanlar:

| Alan | Açıklama |
| --- | --- |
| `status` | `manifest` varsa son durum: `complete`, `partial`, `failed` veya `skipped`. Yoksa `success`, `completed_with_warnings` veya `completed_with_errors`. Desteklenen dosya bulunmaması da `skipped` olur. |
| `llm` | Çözümlenen LLM kimliği. Normalleştirilmiş `model` daima bulunur; `provider` yalnızca adlandırılmış yapılandırılmış sağlayıcıda bulunur. |
| `message` | İsteğe bağlı, okunabilir özet; ör. `"No comments generated. Looks good to me."`. |
| `summary` | İsteğe bağlı toplamlar: `files_reviewed`, `comments`, `total_tokens`, `input_tokens`, `output_tokens`, `cache_read_tokens` (boşsa atlanır), `cache_write_tokens` (boşsa atlanır), `elapsed`. `skipped` çalıştırmalarda yoktur. |
| `comments` | Her zaman vardır; boş olabilir. Yorum alanları yukarıdaki örnektedir. |
| `warnings` | Alt ajan hatalarında bulunur; etkilenen dosya ve hatayı açıklar. |
| `session_id` | Kalıcı kaydedilen incelemelerde bulunur; uyumlu incelemeye devam etmek için `ocr review --resume <session-id>` komutuna verin. |
| `resume` | Devam edilen çalıştırmalarda `resumed_from`, `reused_files`, `rerun_files`, `previous_model`, `current_model` içerir. |

İncelemeye uygun dosya yoksa “değişiklik yok” ile “bulgu yok” ayrılabilsin diye `skipped` nesnesi döner:

```json
{
  "status": "skipped",
  "message": "No supported files changed.",
  "llm": {
    "provider": "anthropic",
    "model": "claude-opus-4-6"
  },
  "comments": []
}
```

### Çıkış kodları {#exit-codes}

| Kod | Anlamı |
| --- | --- |
| `0` | İnceleme tamamlandı veya atlandı; sıfır yorum ya da işlemi durdurmayan uyarılar olabilir. Yalnızca yapılandırılmış bütçe sınırından kaynaklanan kısmi inceleme de `0` döndürür. |
| `1` | Kritik hata veya zaman aşımı, sağlayıcı hatası, yanlış yapılandırma gibi grup başına işletim hatası. CLI hatayı döndürmeden önce mevcut sonuçları yazdırır. |

Uyarılar metin içinde gösterilir; JSON'da `warnings` dizisine eklenir. Etkilenen dosyalar ve hata sınıfları için `manifest.coverage.failed` alanını inceleyin.

## `ocr scan` {#ocr-scan}

Git farkı olmadan tam dosya incelemesi yapar. Her dosyanın çalışma ağacındaki güncel içeriği LLM'e gönderilir. Tanımadığınız kod tabanını veya anlamlı farkı olmayan dizini denetlemek için kullanışlıdır.

```text
ocr scan [flags]
ocr s      [flags]   (alias)
```

`--path` verilmezse tüm depo taranır.

### Seçenekler {#flags-2}

| Seçenek | Kısa | Varsayılan | Açıklama |
| --- | --- | --- | --- |
| `--path <list>` | - | Tüm depo | Depoya göre virgülle ayrılmış dosya/dizinler; ör. `internal/agent`, `internal/llm/client.go`. Çalışma ağacına göre çözümlenir. Tam yazılan yol, yalnızca harf büyüklüğü farklı kardeşinden önceliklidir. Dosya sisteminin farklı harf büyüklüğüyle çözümlediği yol (Windows, varsayılan macOS) yine izlenen dosyaları seçer. Mevcut yola karşılık gelmeyen seçim boş tarama değil hata üretir. |
| `--exclude <patterns>` | - | - | Virgülle ayrılmış gitignore biçiminde dışlamalar; ör. `**/generated/*,*.pb.go`. `rule.json` dışlamalarıyla birleşir. |
| `--output <path>` | `-o` | stdout | UTF-8 dosyaya yazar; `-`, stdout demektir. İlk yazmada oluşturulur, başarısız çalıştırma mevcut dosyaya dokunmaz. Metin çıktısından ANSI renkleri kaldırılır. |
| `--preview` | `-p` | `false` | LLM çağırmadan dosyaları listeler ve filtreler; incelenebilir/dışlanan sayıları, toplam satırları ve dosya başına nedenleri gösterir. JSON desteklenir; SARIF desteklenmez. |

```bash
ocr scan --preview                              # see what would be scanned
ocr scan --path internal/agent                  # scan one directory
ocr scan --path internal/agent,internal/llm/client.go
ocr scan --exclude '**/generated/*,*.pb.go'
```

Tam seçenek listesi için `ocr scan -h` kullanın.

## `ocr session` {#ocr-session}

`~/.opencodereview/sessions/` altındaki günlükleri listeler ve inceler. Oturum kimliğini bulmak, dosya kontrol noktalarını görmek ve yarım kalan incelemeyi sürdürmek için kullanın.

```text
ocr session <sub-command>
ocr sessions <sub-command>   (alias)

Sub-commands:
  list, ls        List recent review sessions for the current repo
  show <id>       Show one session's metadata and per-file items
  comments <id>   Show the review comments recorded in one session
```

### `ocr session list` {#ocr-session-list}

```bash
ocr session list
ocr session list --limit 50
ocr session list --json
ocr session list --branch feat/my-feature --limit 5
ocr sessions ls --repo /path/to/repo --branch main --json
```

| Seçenek | Varsayılan | Açıklama |
| --- | --- | --- |
| `--repo <path>` | Geçerli dizin | Oturumları listelenecek depo. |
| `--json` | `false` | Özetleri JSON olarak üretir. |
| `--limit <n>` | `20` | Listelenen oturum sınırı; `0` sınırsız. |
| `--branch <name>` | Boş | Kayıtlı dal adı, harf büyüklüğü dahil tam eşleşen oturumlar. |

Dal filtresi `--limit` öncesinde uygulanır ve yeniden eskiye sıralamayı korur. Verilmezse veya boşsa filtrelenmemiş davranış sürer. Filtre varken kayıtlı dalı olmayan oturumlar dışlanır; eşleşme yoksa metinde dala özgü mesaj, JSON'da `[]` üretilir.

Kayıtlı üst veriler kullanıldığından silinmiş dal da checkout yapılmadan aranabilir. `--repo` dizini mevcut olmalıdır ama artık Git deposu olmak zorunda değildir. Filtre, devam ettirme kontrollerini değiştirmez.

### `ocr session show` {#ocr-session-show}

Varsayılan olarak kimliği tüm kayıtlı depolarda arar. Aramayı geçerli dizinle sınırlamak için `--repo .` verin.

Genel aramada benzersiz kimlik, çalışma ağacı değiştikten veya özgün checkout silindikten sonra da incelenebilir. Kimlik birden fazla depoda varsa adaylar listelenir ve `--repo` ile seçim gerekir. `--repo` mevcut dizin gerektirdiğinden özgün checkout'u silinmiş, birden fazla depoda bulunan oturumu bu şekilde seçemezsiniz.

Devam edilen çalıştırma, önceki çalıştırmayı ve varsa sağlayıcı/model geçişini de gösterir.

```bash
ocr session show <session-id>
ocr session show --json <session-id>
ocr session show --repo . <session-id>
ocr session show --repo /path/to/repo <session-id>
```

| Seçenek | Varsayılan | Açıklama |
| --- | --- | --- |
| `--repo <path>` | Tüm depolar | Aramayı mevcut depo diziniyle sınırlar. |
| `--json` | `false` | Üst verileri ve dosya girdilerini JSON üretir. |

### `ocr session comments` {#ocr-session-comments}

Oturumdaki tüm kalıcı yorumları `ocr review` terminal biçimiyle yazdırır: yol, satır aralığı, önem rozeti ve öneri farkı.

```bash
ocr session comments <session-id>
ocr session comments --json <session-id>
ocr session comments --severity high <session-id>
ocr session comments --severity critical,high --category bug,security <session-id>
```

| Seçenek | Varsayılan | Açıklama |
| --- | --- | --- |
| `--repo <path>` | Geçerli dizin | İncelenecek oturumun deposu. |
| `--json` | `false` | Yorumları JSON dizisi olarak üretir. |
| `--severity <list>` | Tümü | Virgülle ayrılmış önemler: `critical`, `high`, `medium`, `low`. |
| `--category <list>` | Tümü | Virgülle ayrılmış kategoriler; ör. `bug`, `security`. |

### `ocr session compare` {#ocr-session-compare}

İki oturumun bulgularını dört gruba ayırır: **yeni** (yalnızca sonraki), **süren** (ikisinde de), **giderilen** (yalnızca önceki) ve **incelenmeyen** (öncekinde bulunan ancak sonraki oturumun incelemediği dosyalardaki bulgular; giderilmiş sayılmaz).

Eşleştirme satır numarasıyla değil yol, kategori ve sorunlu kod parçasıyla yapılır. Dosyada aşağı taşınan bulgu sürüyor sayılır. Sonraki bildirim yeniden adlandırma kaydetmişse eşleştirmeden önce eski yol yeniye dönüştürülür.

```bash
ocr session compare <before-session-id> <after-session-id>
ocr session diff <before-session-id> <after-session-id>
ocr session compare --json <before-session-id> <after-session-id>
```

İki oturum aynı depoya ait olmalıdır; aksi halde komut hata verir. Farklı inceleme kipleri yalnızca stderr'e uyarı yazar; JSON başka komuta aktarılabilir kalır.

| Seçenek | Varsayılan | Açıklama |
| --- | --- | --- |
| `--repo <path>` | Geçerli dizin | Karşılaştırılacak oturumların deposu. |
| `--json` | `false` | `new`, `persisting`, `resolved`, `not_reviewed` alanlarıyla JSON üretir. |

### `ocr session export` {#ocr-session-export}

Tek oturumu bağımsız HTML dosyasına dönüştürür. Stil ve betik gömülü olduğundan ağ erişimi olmadan `file://` ile açılır; CI incelemeyi derleme çıktısı olarak arşivleyebilir.

```bash
ocr session export -o review.html
ocr session export 20250601-100000-abc123 -o review.html
```

Kimlik verilmezse deponun en yeni oturumu dışa aktarılır. Başarılı `ocr review` kimliği yazdırmadığı için varsayılan budur. `-o` verilmezse HTML stdout'a gider.

Sayfa, oturumun kaydettiği kaynak parçalarını içerir. Yayımlamadan önce dosyayı deponun kendisiyle aynı gizlilik düzeyinde değerlendirin.

| Seçenek | Varsayılan | Açıklama |
| --- | --- | --- |
| `--repo <path>` | Geçerli dizin | Oturumu dışa aktarılacak depo. |
| `--output <path>`, `-o` | stdout | HTML'i dosyaya yazar. |

### `ocr session rm` {#ocr-session-rm}

`~/.opencodereview/sessions/` içindeki tek bir oturumu, varsa ilişkili analiz raporu, kanıtlar ve günlüklerle birlikte siler.

```bash
ocr session rm 9f2c1b4a-7e35-4d61-b2f0-6c8a41d9e72b
ocr session rm 9f2c1b4a-7e35-4d61-b2f0-6c8a41d9e72b --yes
ocr session rm 9f2c1b4a-7e35-4d61-b2f0-6c8a41d9e72b --repo ~/work/my-project
```

Kimlik tek başına yeterlidir; herhangi bir dizinden çalışır. Aynı kimlik birden fazla depoda kayıtlıysa adaylar listelenir ve hiçbir şey silinmez. `--repo` ile seçin.

Depo, dal, başlangıç zamanı, dosya ve yorum sayıları gösterilip onay istenir. **Etkileşimsiz stdin hayır sayılır.** Bu nedenle CI veya boru hattında istemi atlamak için `--yes` (`-y`) gerekir.

Üst verileri ayrıştırılamayan oturum silinebilir; hiç okunamayan oturum ise bildirilir. `--repo` kullanıldığında farklı depo kaydeden veya depo kaydı olmayan oturum reddedilir; onu yalnızca kimlikle silin.

| Seçenek | Varsayılan | Açıklama |
| --- | --- | --- |
| `--repo <path>` | Tüm depolar | Yalnızca bu depoda arar. |
| `--yes`, `-y` | `false` | Onay istemini atlar. |

## `ocr rules` {#ocr-rules}

Kuralları incelemek içindir; tek alt komutu vardır:

```text
ocr rules check [flags] <file-path>

Flags:
  --repo <path>    Git repository root (default: current dir)
  --rule <path>    Path to a custom rule JSON file
```

Verilen dosya yolu için OCR:

1. Dört katmanı dolaşır: özel → proje → genel → sistem.
2. İlk eşleşmeyi seçer.
3. **Kaynak katmanı**, eşleşen **glob kalıbını** ve çözümlenen **kural metnini** yazdırır.

```bash
$ ocr rules check src/main/java/com/example/Foo.java
File: src/main/java/com/example/Foo.java
Source: System built-in
Pattern: **/*.java
Rule:
────────────────────────────────────────
<contents of internal/config/rules/rule_docs/java.md>
────────────────────────────────────────
```

“Özel kuralım neden çalışmıyor?” sorusunu çözmek için yararlıdır. Tam öncelik açıklaması: [İnceleme kuralları](../review-rules/).

## `ocr config` {#ocr-config}

Anahtarları `~/.opencodereview/config.json` dosyasına kaydeder ve etkileşimli kurulum sunar. Dört alt komut:

```text
ocr config set <key> <value>
ocr config unset <key>                     Clear a saved key
ocr config provider                        Interactive provider setup
ocr config model                           Interactive model selection
```

- **`set`** — Tek ayarı etkileşimsiz yazar. `effort`, `low` / `medium` / `high` kabul eder ve her çalıştırmanın varsayılanını belirler; `--effort` tek çağrıda önceliklidir.
- **`unset`** — Kayıtlı anahtarı temizler. `provider`, `max_tokens`, `effort`, `custom_providers.<name>` ve `mcp_servers.<name>` desteklenir. `effort` temizlenirse `medium` döner. Etkin sağlayıcı silinirse `provider` ve `model` temizlenir; yenisini `ocr config provider` ile seçin.
- **`provider`** — Etkileşimli kurulumu açar; ek argüman almaz. Etkileşimsiz kullanım: `ocr config set provider <name>`.
- **`model`** — Etkileşimli model seçimini açar; ek argüman almaz. Etkileşimsiz kullanım: `ocr config set model <name>`.

Tüm anahtarlar, şemalar ve örnekler için [Yapılandırma](../configuration/) belgesine bakın.

## `ocr auth` {#ocr-auth}

`ocr auth login`, `status` ve `logout` için varsayılan ChatGPT ile giriştir. Açık `--provider chatgpt` desteklenir.

`chatgpt` sağlayıcısıyla belgelenen giriş akışını kullanın; bkz. [ChatGPT önizleme kılavuzu](../chatgpt-preview/).

```bash
ocr auth login --provider chatgpt
ocr auth login --provider chatgpt --no-browser
ocr auth login --provider chatgpt --enable-plan
ocr auth login --provider chatgpt --new-account
ocr auth status --provider chatgpt
ocr auth select <saved-client-id> --provider chatgpt
ocr auth logout --provider chatgpt --account <saved-client-id>
ocr llm models
```

`ocr auth`, `--provider chatgpt` ve `--account <client-id>` kabul eder. `login` ayrıca `--no-browser`, `--new-account`, `--enable-plan` kabul eder. ChatGPT `--device` desteklemez. `--enable-plan`, kayıtlı kayıt için onay ister; `--new-account` ile birleşemez. Model listesi hesaba özgüdür; listede olmasa da açık model kabul edilir. `extra_body` yalnızca `reasoning`, `text`, `prompt_cache_key` için desteklenir. Hesap yönetimi, izinler ve kısıtlar için [önizleme kılavuzuna](../chatgpt-preview/) bakın.

## `ocr llm` {#ocr-llm}

Üç LLM yardımcı alt komutu vardır:

```text
ocr llm <sub-command>

Sub-commands:
  test         Send a test conversation to the configured LLM model
  providers    List all built-in LLM providers
  models       List current account-specific ChatGPT models
```

### `ocr llm test` {#ocr-llm-test}

```text
ocr llm test
```

Uç noktayı `ocr review` ile aynı şekilde çözümler; [`internal/config/testconnection/task.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/testconnection/task.json) içindeki hazır sohbet isteğini bir kez gönderir ve şunları yazdırır:

```
Source: <which strategy was used>
URL:    <endpoint URL>
Model:  <effective model>
<the model's reply>
✓ Connection test successful
```

Sıfırdan farklı çıkış, eksik uç nokta yapılandırması veya istek hatası (ağ/kimlik doğrulama/model) demektir. Hata mesajı hangisi olduğunu belirtir.

### `ocr llm providers` {#ocr-llm-providers}

```text
ocr llm providers
```

Yerleşik sağlayıcıları üç sütunlu tabloda listeler:

```
Built-in providers:
  NAME        PROTOCOL    BASE URL
  ----        --------    --------
  anthropic   anthropic   https://api.anthropic.com
  …
```

Ardından etkileşimli `ocr config provider` veya etkileşimsiz `ocr config set provider <name>` ile yapılandırma ipucu gösterilir.

## `ocr viewer` {#ocr-viewer}

```text
ocr viewer [flags]

Flags:
  --addr <address>   listen address (default: localhost:5483)
  --open <mode>      when to open the browser: auto, always, never (default: auto)

Examples:
  ocr viewer                     # start and open the browser
  ocr viewer --addr :3000        # bind to all interfaces on port 3000
  ocr viewer --open=never        # just print the URL
  ocr viewer --open=always       # force it when auto declines (piped output, WSL)
```

`~/.opencodereview/sessions/...` dosyalarını okuyup geçmiş incelemeleri tarayıcı arayüzünde gösteren gömülü HTTP sunucusunu başlatır. Bkz. [Oturum görüntüleyicisi](../viewer/).

`--open=auto`; stdout terminal değilse, `SSH_CONNECTION` ayarlı olup ekran yönlendirme yoksa veya Linux'ta `DISPLAY` ve `WAYLAND_DISPLAY` ikisi de yoksa tarayıcıyı açmaz. Neden URL yanında gösterilir. Otomatik kip açmasa da tarayıcı erişilebiliyorsa `--open=always` kullanın.

## `ocr version` {#ocr-version}

```text
ocr version
ocr --version
ocr -V
```

Derleme sırasında işlenen sürümü, varsa kısa Git commit'ini, platformu (`<GOOS>/<GOARCH>`), varsa derleme tarihini ve GitHub adresini (`https://github.com/alibaba/open-code-review`) yazdırır.

## ocr completion {#ocr-completion}

Komut, seçenek ve argümanları Tab ile tamamlayabilmek için kabuk tamamlama betikleri üretir.

### Bash {#bash}

Yalnızca mevcut oturum için:

```bash
source <(ocr completion bash)
```

Kalıcı (Linux):

```bash
ocr completion bash > /etc/bash_completion.d/ocr
```

Kalıcı (macOS):

```bash
ocr completion bash > $(brew --prefix)/etc/bash_completion.d/ocr
```

### Zsh {#zsh}

Tamamlama etkin değilse bir kez etkinleştirin:

```bash
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

Ardından kalıcı olarak yükleyin:

```bash
ocr completion zsh > "${fpath[1]}/_ocr"
```

Geçerli olması için yeni kabuk başlatın.

### Fish {#fish}

Yalnızca mevcut oturum için:

```bash
ocr completion fish | source
```

Kalıcı:

```bash
ocr completion fish > ~/.config/fish/completions/ocr.fish
```

### PowerShell {#powershell}

Yalnızca mevcut oturum için:

```powershell
ocr completion powershell | Out-String | Invoke-Expression
```

Kalıcı kullanım için betiği bir kez üretip profilinizden yükleyin:

```powershell
ocr completion powershell > ocr.ps1
```

PowerShell profilinize `ocr.ps1` dosyasını dot-source eden bir satır ekleyin.

## İpuçları ve dikkat edilecekler {#tips-gotchas}

- `--audience agent`, `--format json` anlamına gelmez; `--format json` da terminali sessizleştirmez. Biri ilerleme görünümünü, diğeri çıktı yapısını denetler. JSON tek başına stderr ilerlemesini korur; kaldırmak için `--audience agent` ekleyin.
- `--background`, inceleme kalitesini yönlendiren önemli bir seçenektir. Başka ajandan çağırırken gereksinimi/PR açıklamasını verin.
- Farkı tek başına `MAX_TOKENS` değerinin (varsayılan `200000`) %80'ini aşan dosya LLM çağrısından önce atlanır. Günlüğe yazılır ancak çalıştırmayı başarısız kılmaz.
- `MAX_TOKENS` yalnızca **istemi** sınırlar. Çıktı ayrı `MAX_COMPLETION_TOKENS` (`16384`) ile sınırlıdır; büyük bağlamlı model için `--max-tokens` artırmak çıktı maliyetini artırmaz.
- Değişen satırlar hem `PLAN_MODE_LINE_THRESHOLD` (gruptaki en büyük dosyada `50`) hem `PLAN_MODE_GROUP_LINE_THRESHOLD` (2+ dosyalı grubun toplamında `100`) altında kalırsa planlama **otomatik atlanır**.
- İnceleme öncesi yalnızca üst veri taşıyan tek düşük maliyetli LLM çağrısıyla dosyalar anlamsal gruplanır. İşleyici + servis + test gibi ilişkili değişiklikler ortak konuşmada incelenir. Gruplama başarısızsa dosya başına bir gruba dönülür.
- İnceleme yüzeysel geliyorsa `--effort high` kolay bir kalite ayarıdır. Hızlı genel kontrol istiyorsanız `--effort low`, varsayılana göre maliyeti kabaca yarıya indirir.

## Ayrıca bkz. {#see-also}

- [Hızlı başlangıç](../quickstart/) — Kurulum ve ilk inceleme.
- [Yapılandırma](../configuration/) — Seçeneklerin arkasındaki ortam değişkenleri ve anahtarlar.
- [İnceleme kuralları](../review-rules/) — `--rule` ve kural çözümleme.
- [Entegrasyonlar](../integrations/agent-skill/) — Ajanlardan ve CI'dan `ocr review` çağırma.

---
title: Katkıda bulunma
sidebar:
  order: 13
---

OCR, Apache-2.0 lisanslı açık kaynak bir projedir. Hata bildirimleri, belge düzeltmeleri ve kod katkıları memnuniyetle karşılanır. Bu sayfa kısa bir özettir. Katkı kılavuzunun tamamı için [Türkçe çeviriye](https://github.com/alibaba/open-code-review/blob/main/docs/i18n/CONTRIBUTING.tr-TR.md) veya [İngilizce aslına](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md) bakın.

## Katkı yolları {#ways-to-contribute}

Katkı için Go yazmanız gerekmez:

- **Hata bildirimleri** — yeniden oluşturma adımlarıyla [GitHub kaydı](https://github.com/alibaba/open-code-review/issues/new/choose) açın.
- **Özellik istekleri** — [Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas) altında konu veya özellik isteği kaydı açın.
- **Belgeler** — yazım hataları, eksik örnekler, bozuk bağlantılar; bu PR'lar genellikle en hızlı birleşir.
- **Diğer PR'ları inceleme** — maintainer olmayan kişilerin yorumları da inceleme yükünü azaltır.
- **Kod** — hata düzeltmeleri, performans çalışmaları, yeni özellikler.

## Yerel geliştirme ortamı {#local-development-setup}

### Ön koşullar {#prerequisites}

- [Go ≥ 1.26.10](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### Kaynağı edinme {#getting-the-source}

```bash
# Fork on GitHub, then:
git clone https://github.com/<your-username>/open-code-review.git
cd open-code-review
git remote add upstream https://github.com/alibaba/open-code-review.git

make build       # writes dist/opencodereview (dist/opencodereview.exe on Windows)
make test        # LC_ALL=C go test -v -race -count=1 ./...
```

> `upstream` uzak deposu salt okunurdur. `origin` üzerine (kendi fork'unuza) gönderip oradan PR açın.

### Yerel derlemeyi çalıştırma {#running-your-local-build}

```bash
./dist/opencodereview review --preview        # macOS / Linux
.\dist\opencodereview.exe review --preview   # Windows (PowerShell / cmd)
```

Kolaylık için `~/bin/ocr-dev` altında `dist/opencodereview` dosyasına sembolik bağlantı oluşturun; böylece her depodan `ocr-dev` çağırabilirsiniz.

### Make hedefleri {#make-targets}

| Hedef | İşlev |
|---|---|
| `make build` | Geçerli platform için `dist/opencodereview` dosyasını derler (Windows'ta `dist/opencodereview.exe`). |
| `make build-darwin-amd64` | macOS Intel için çapraz derleme. |
| `make build-darwin-arm64` | macOS Apple Silicon için çapraz derleme. |
| `make build-linux-amd64` | Linux x86_64 için çapraz derleme. |
| `make build-linux-arm64` | Linux ARM64 için çapraz derleme. |
| `make build-windows-amd64` | Windows x86_64 için çapraz derleme. |
| `make build-windows-arm64` | Windows ARM64 için çapraz derleme. |
| `make build-all` | Altı ikili dosyanın tümü (linux/darwin/windows × amd64/arm64). |
| `make sha256sum` | Derleme çıktıları için `sha256sum.txt` üretir. |
| `make dist` | `clean → build-all → sha256sum`; CI bunu çalıştırır. |
| `make test` | Yarış durumu algılayıcısıyla testleri çalıştırır. |
| `make clean` | `dist/` dizinini kaldırır. |

## Dal ve commit kuralları {#branching-and-commit-conventions}

### Dal önekleri {#branch-prefixes}

| Önek | Amaç |
|---|---|
| `feat/` | Yeni özellik |
| `fix/` | Hata düzeltmesi |
| `docs/` | Yalnızca belge |
| `refactor/` | Davranışı değiştirmeyen yeniden düzenleme |
| `test/` | Yalnızca test değişiklikleri |
| `chore/` | Derleme / CI / araçlar |

```bash
git checkout main
git pull upstream main
git checkout -b feat/anthropic-streaming
```

### Commit mesajları {#commit-messages}

[Conventional Commits](https://www.conventionalcommits.org/) biçimi:

```
<type>(<scope>): <short summary>

[optional body explaining the why]
```

Örnekler:

```
feat(viewer): add session search
fix(llm): handle timeout errors in Anthropic API calls
docs(readme): clarify endpoint resolution priority
refactor(viewer): extract task-card rendering into helper
```

Otomatik değişiklik günlüğünün düzenli olması için **PR başlıklarında** da aynı biçim kullanılır.

`PR Commit Policy`, PR başlığını ve gövde dâhil her commit mesajının tamamını büyük/küçük harf ayırmadan denetler. `[skip ci]`, `[ci skip]`, `[no ci]`, `[skip actions]`, `[actions skip]` veya `skip-checks: true` / `skip-checks:true` satırı kullanmayın.

Hataları gidermek için PR başlığını düzenleyin veya katkı dalında ilgili commit mesajlarını yeniden yazın. Temiz bir commit eklemek önceki ihlalleri kaldırmaz. PR açıklamaları bu talimatlardan söz edebilir; açıklamalar denetlenmez.

## Proje yapısı {#project-layout}

```
open-code-review/
├── acp/                       # Independent ACP (Agent Client Protocol) adapter for the CLI
├── cmd/opencodereview/        # CLI entry point — flag parsing, dispatch
├── docs/                      # Localized README and contributing guides
├── imgs/                      # README images and logos
├── internal/
│   ├── agent/                 # Review agent logic, sub-agent dispatch
│   ├── chatgptauth/           # ChatGPT OAuth and credential management
│   ├── config/                # Template, rules, allowlist, embedded JSON
│   ├── delegate/              # Review specification generation without LLM calls
│   ├── diff/                  # Git diff parsing
│   ├── gitcmd/                # Git subprocess runner
│   ├── llm/                   # LLM clients and endpoint resolution
│   ├── llmloop/               # LLM tool-use loop shared by review and scan
│   ├── mcp/                   # MCP clients and tool adapters
│   ├── model/                 # Data structs (LlmComment, Diff, …)
│   ├── pathutil/              # Path utilities
│   ├── release/               # Release artifact naming consistency checks
│   ├── scan/                  # Full-file code review
│   ├── session/               # JSONL session writer
│   ├── stdout/                # Quiet-able stdout writer
│   ├── suggestdiff/           # Suggestion diff rendering
│   ├── telemetry/             # OpenTelemetry config + helpers
│   ├── tool/                  # Tool registry + provider impls
│   └── viewer/                # Embedded HTTP UI for browsing review sessions
├── pages/                     # Website landing page and docs (separate React app)
├── plugins/                   # Coding agent integrations (Claude Code, Codex, OpenCode, …)
├── extensions/                # VS Code and IntelliJ IDEA extensions, shared frontend
├── examples/                  # CI recipes (GitHub Actions, GitLab CI)
├── skills/                    # Review and delegation skill definitions
├── scripts/                   # Install, publish, and repository check scripts
├── npm/                       # Per-platform optional dependency packages
└── bin/                       # NPM wrapper (Node)
```

Katkıların çoğu `internal/agent/`, `internal/tool/` veya `internal/llm/` dizinlerini etkiler. `cmd/opencodereview/` içindeki CLI katmanı bilinçli olarak küçüktür: seçenek çözümleme ve komut yönlendirme.

## Yapay zekâ destekli geliştirme {#ai-assisted-development}

Geliştirmede yapay zekâ desteği kullanmanız memnuniyetle karşılanır; katkıyı kolaylaştırması sevindiricidir. Kabul edilemeyen, model çıktısının okunmadan, fazlalıkları ve hataları temizlenmeden doğrudan commit'e girmesidir. Böyle gönderimler inceleme konuşmasını yavaşlatır ve PR'ın ilerlemesini zorlaştırır.

Çalışmanızda yapay zekâ kullandıysanız aşağıdaki kurallara uyun.

### Kurallar: {#rules}

1. **İlk iş kaydında veya PR'da AI/LLM kullandığınızı, kullanılan araç ve modelleri açıklamalısınız.**
2. Yapay zekânın yazdığı her kod satırını anlamalı, ne yaptığını bilmelisiniz.
3. İnceleyici değişikliğin nedenini sorduğunda, kodu siz veya yapay zekâ yazmış olsa da kendiniz açıklayabilmelisiniz. Maintainer sorularına ve inceleme yorumlarına yanıtlarınızın özü kendi anlayışınızdan gelmelidir. AI/LLM yalnızca çeviri veya anlatımı iyileştirmek için kullanılabilir; sizin yerinize yanıt üretmek için kullanılamaz.
4. PR, `AI generated -> fixed -> fixed -> fixed` gibi tekrar eden döngüler içermemelidir. Bu, çıktıyı incelemek yerine her sorun çıktığında yapay zekâya yeniden düzelttirdiğinizi gösterebilir.
5. Herhangi bir üyeden inceleme istemeden önce AI/LLM tarafından üretilen tüm kodu, metni ve diğer içerikleri kendiniz incelemelisiniz.
6. Commit'leri “Assisted-by”, “Co-developed-by” veya benzeri son bilgiler dâhil AI/LLM'ye atfetmemelisiniz.
7. Aşırı uzun commit mesajları yazmayın. Önemli bilgileri daraltılmış commit mesajları yerine PR açıklamasına yazın.
8. Bunların tümünü yapamıyor veya yapmak istemiyorsanız iş kaydınızı ya da PR'ınızı kapatın.

Teşekkürler!

## Lisans başlıkları {#license-headers}

Her kaynak dosyası (`.go`, `.js`, `.mjs`, `.ts`, `.tsx`, `.kt`, `.kts`, `.sh`, `.py`, `.css`) SPDX lisans başlığı içermelidir. Yeni dosya oluşturduktan sonra çalıştırın:

```bash
make license-add
```

Gerekli başlık otomatik eklenir. CI, başlığı eksik PR'ları reddeder.

## Kod kalitesi kontrolleri {#code-quality-checks}

PR açmadan önce:

```bash
make check      # format, lint, and verify license headers
make test       # race-enabled, runs in CI on every push
make build      # smoke test the binary builds
```

CI her gönderimde aynı kontrolleri çalıştırır.

## Yeni araç ekleme {#adding-new-tools}

Bu bölüm çalışma zamanı yapılandırmasını değil, yerleşik ajanın kaynak kodu değişikliklerini kapsar. Temel araç değişikliklerini testler ve karşılaştırmalarla doğrulayın. Haricî iş bağlamı için [MCP sunucularını](../mcp/) kullanın.

Araç iki parçadan oluşur:

1. [`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json) içindeki **JSON tanımı**: LLM'nin gördüğü ad, açıklama ve JSON şemalı parametreler.
2. `internal/tool/definitions.go` içinde kaydedilen, gerçek uygulamayı içeren **Go sağlayıcısı**.

Yeni araç adının çalışması için ikisi de bulunmalıdır. Mevcut altı araç için [Araçlar](../tools/) sayfasına bakın ve bunları şablon alın.

Aracı ilgili her şablonun `MAIN_TASK.tools` dizisine ekleyin. İnceleme ve taramanın listeleri ayrıdır; sıra model isteğini etkilediği için sabit tutun. Planlama sırasında aracı tanıtıp çağrılarını açmamak için `PLAN_TASK.tool_references` kullanın. Araç tanımları görev üyeliği bayrakları içermemeli; tanımdaki ad katalog girdisiyle eşleşmelidir.

Bu listeler model istekleri için tanım seçer; yürütme yalıtımı sağlamaz. MCP keşfi ve incelemeye özgü tamamlama/filtreleme protokolleri mevcut görev bağdaştırıcılarında kalır. Diğer yardımcı görevler katalog bağlarını kullanmaz; desteklenmeyen `tools` veya `tool_references` alanları sessizce yok sayılmak yerine reddedilir.

## Yeni kural deseni ekleme {#adding-new-rule-patterns}

Yeni glob desenini kural belgesine bağlamak için `internal/config/rules/system_rules.json` dosyasını düzenleyin; ilgili Markdown'ı `internal/config/rules/rule_docs/` altına ekleyin. Her desenin tek İngilizce kural dosyası vardır. `language` ayarı yalnızca sistem istemine modelin belirtilen dilde yanıt vermesini isteyen yönerge ekler; kural dosyalarını değiştirmez.

## PR süreci {#pr-process}

1. **Büyük değişikliklerde önce iş kaydı açın.** Yaklaşım üzerinde önceden uzlaşmak, kod incelemesinde uyuşmazlık fark etmekten iyidir.
2. **PR başına tek mantıksal değişiklik.** İlişkisiz iki düzeltme için iki PR gönderin.
3. **Testleri güncelleyin.** Davranış değişiklikleri test kapsamı gerektirir; `make test` geçmelidir.
4. **Belgeleri güncelleyin.** Seçenekleri, yapılandırma anahtarlarını veya inceleme sürecini etkileyen değişikliklerde hem bu belge sitesini ([`docs/`](https://github.com/alibaba/open-code-review) içinde) hem ilgili satır içi yardımı güncelleyin.
5. **PR şablonunu doldurun.** Bir maintainer genellikle birkaç iş günü içinde inceler.

## Daha hızlı PR incelemesi için ipuçları {#tips-for-faster-pr-reviews}

- **CLA'yı erkenden imzalayın.** İlk katkılarda CLA botu yorumu sıkça gözden kaçar. Bot istediğinde Katkıcı Lisans Sözleşmesi'ni imzalayın; imza olmadan PR birleşemez.
- **Tüm CI kontrolleri geçsin.** Kontrolleri başarısız PR'lar incelenmez. Göndermeden önce `make test` ve `make build` çalıştırın.
- **Değişiklikler küçük ve odaklı olsun.** Tek işi iyi yapan PR daha kolay incelenir, daha hızlı ilerler ve daha az düzeltme turu gerektirir.
- **Açık ve doğru açıklama yazın.** Neyin neden değiştiğini anlatın. Açıklama gerçek farkla eşleşmelidir; uyuşmazlık güveni azaltır. Kapsam değiştiyse inceleme istemeden önce açıklamayı güncelleyin.
- **Davranış değişikliklerini test edin.** Testsiz özellik ve düzeltmeler soru yaratır; testler doğruluğu ve amaçlanan davranışı gösterir.
- **Mevcut kod düzenine uyun.** Çevredeki kodun biçimi, adlandırması ve mimarisini izleyin. Tutarlılık inceleme yükünü ve yalnızca biçimle ilgili yorumları azaltır.
- **Geri bildirime zamanında yanıt verin.** İstenen değişiklikleri hızla ele alın. Katılmıyorsanız yorumu yok saymak yerine gerekçenizi açıklayın.

## Katkıcı Lisans Sözleşmesi (CLA) {#contributor-license-agreement-cla}

Proje Alibaba Open Source CLA gerektirir. İlk PR'da bot bağlantı gönderir; elektronik olarak imzalayın (yaklaşık bir dakika sürer). Sonraki PR'larda yeniden imza gerekmez.

## İlk katkınız mı? {#first-contribution}

[`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue) veya [`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted) etiketli kayıtlara bakın. Çoğu küçük ve bağımsızdır; başlamak için yeterli bağlam içerir.

## Ayrıca bakın {#see-also}

- [Mimari](../architecture/) — `internal/agent/` üzerinde çalışmadan önce gereken genel bakış.
- [Araçlar](../tools/) — mevcut araçların yapısı.
- [Türkçe katkı kılavuzunun tamamı](https://github.com/alibaba/open-code-review/blob/main/docs/i18n/CONTRIBUTING.tr-TR.md)

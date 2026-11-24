---
title: CI/CD
sidebar:
  order: 4
---

OCR'ı her Pull Request veya Merge Request üzerinde çalıştırın. Ana depo, kopyalayıp yapılandırabileceğiniz iki hazır iş akışı sunar: GitHub Actions ve GitLab CI. İkisi de [Komut satırı başvurusu](../../cli-reference/#json) belgesindeki çekirdek komutun etrafındaki ince bir entegrasyon katmanıdır.

## CI/CD entegrasyonu nasıl çalışır? {#how-ci-cd-integration-works}

Bu sayfadaki tüm örnekler aynı düzeni izler. Aşağıdaki GitHub Actions ve GitLab CI bölümleri bunun somut uygulamalarıdır:

1. **PR / MR olayıyla tetikleme.** Yeni pull request, güncellenen merge request veya elle yazılan `/open-code-review` yorumu işi başlatır.
2. **Çalıştırıcıya `ocr` yükleme.** Genellikle `npm install -g @alibaba-group/open-code-review` kullanılır. Çalıştırıcı geçici olduğundan her çalıştırmada yüklenir.
3. **CI sırlarından LLM yapılandırma.** `ocr config set` ile uç nokta, token ve model ayarlanır; başvurulabilecek kalıcı `~/.opencodereview` yoktur.
4. **Makine tarafından okunabilir çıktıyla aralık incelemesi.** Böylece stdout temiz bir JSON nesnesi içerir:

```bash
   ocr review \
     --from "origin/<base-branch>" \
     --to "origin/<head-branch>" \
     --format json \
     --audience agent
   ```

`--format json` ayrıştırılabilir çıktı üretir; `--audience agent` ilerleme satırlarını gizler. Örneklerin kullandığı yapı için [JSON çıktısına](../../cli-reference/#json) bakın.

5. **JSON'u ayrıştırıp `comments[]` dizisini dolaşma.**
6. **Yorumları sağlayıcının inceleme API'siyle PR / MR'a gönderme.** Geçerli satır bilgisi olmayan dosya düzeyindeki bulgular satır içine değil, özet notuna eklenir. Toplu satır içi gönderim reddedilirse düz özet yorumuna dönülür.

İki tür kimlik bilgisi kullanılır: bulgu üretmek için OCR'ın kullandığı **LLM kimlik bilgileri** ve yorum göndermek için kullanılan **PR/MR yazma token'ı**. GitHub örneğinde ikincisi `GITHUB_TOKEN` ile otomatik sağlanır. GitLab'da açıkça `GITLAB_API_TOKEN` önerilir; fork MR'larında yedek olarak yerleşik `CI_JOB_TOKEN` kullanılır (`/discussions` üzerinden tartışma gönderebilir). Güvenilirlik için özel token önerilir.

## GitHub Actions {#github-actions}

Kaynak iş akışı: [`examples/github_actions/ocr-review.yml`](https://github.com/alibaba/open-code-review/blob/main/examples/github_actions/ocr-review.yml).

### Ne yapar? {#what-it-does}

- `pull_request_target` (`opened`, `synchronize`, `reopened`) olaylarında **ve** gövdesi `/open-code-review` veya `@open-code-review` ile başlayan `issue_comment` olaylarında çalışır. Yorumla talep üzerine yeniden çalıştırılabilir. Fork PR'larında sırların erişilebilir olması için `pull_request` yerine [`pull_request_target`](https://docs.github.com/en/actions/reference/security/securely-using-pull_request_target) kullanılır; OCR yalnızca farkı okur, PR kodunu yürütmez.
- `npm install -g @alibaba-group/open-code-review` ile yükler, `ocr config set` ile yapılandırır ve dal aralığı kipinde inceleme yapar.
- JSON'u ayrıştırır; GitHub Pull Request Review API ile bulguları satır içi yorum olarak gönderir. Satır bilgisi olmayanları özete ekler. Toplu gönderim başarısızsa tek tek dener ve istatistikleri özet yorumunda gösterir.

### Kurulum {#install}

İş akışını deponuza ekleyin:

```bash
mkdir -p .github/workflows
curl -o .github/workflows/ocr-review.yml \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/github_actions/ocr-review.yml
```

[2 Aralık 2026](https://github.blog/changelog/2026-11-02-pull_request_target-blocked-by-default-december-2/) tarihinden itibaren GitHub'da barındırılan herkese açık depolarda `pull_request_target` olayı, geçerli bir Actions olay politikası açıkça izin vermedikçe varsayılan olarak engellenecek. PR oluşturulduğunda, güncellendiğinde veya yeniden açıldığında OCR'ın otomatik olarak çalışmaya devam etmesi gerekiyorsa deponuzda **Settings → Actions → Policies** bölümünü açın ve [iş akışına uygulanan bir politika oluşturun veya güncelleyin](https://docs.github.com/en/actions/how-tos/administer/control-workflow-execution#creating-a-workflow-execution-policy).

- Yeni politika oluştururken **Target workflows** altında **Add target** düğmesine tıklayın ve `.github/workflows/ocr-review.yml` dosyasını belirtin.
- Mevcut bir politikayı güncellemeden önce yalnızca `.github/workflows/ocr-review.yml` dosyasını hedeflediğini doğrulayın.
- **Restrict events** altında, otomatik incelemeler için `pull_request_target`, yorumla tetiklenen yeniden incelemeler için `issue_comment` olayına izin verin.
- **Enforcement status** ayarını **Active** olarak değiştirin ve politikayı kaydedin.

### Gerekli sırlar {#required-secrets}

**Settings → Secrets and variables → Actions** altında ayarlayın:

| Sır | Zorunlu | Açıklama |
| --- | --- | --- |
| `OCR_LLM_URL` | Evet | LLM API uç noktası; ör. `https://api.openai.com/v1/chat/completions`. |
| `OCR_LLM_AUTH_TOKEN` | Evet | LLM API kimlik doğrulama token'ı. Bu CI sırrı `ocr config set llm.auth_token` komutuna verilir. OCR'ın doğrudan ortam değişkeni `OCR_LLM_AUTH_TOKEN` değil, `OCR_LLM_TOKEN` olur. |
| `OCR_LLM_MODEL` | Hayır | Model adı. Varsayılan yoktur; açıkça ayarlanmalıdır. |
| `OCR_LLM_USE_ANTHROPIC` | Hayır | Anthropic Claude modelleri için `true`. |

`GITHUB_TOKEN` otomatik sağlanır. İş akışı yorum gönderebilmek için `pull-requests: write` bildirir.

> İş akışı başlangıçta `ocr config set llm.extra_body '{"thinking": {"type": "disabled"}}'` çalıştırarak düşünme kipini kapatır. Sağlayıcınız düşünmenin açık kalmasını gerektiriyorsa bu satırı kaldırın.

### Action girdileri {#action-inputs}

Kaynak iş akışı, `uses: alibaba/open-code-review@main` üzerinden yeniden kullanılabilir bileşik action'a ([`action.yml`](https://github.com/alibaba/open-code-review/blob/main/action.yml)) başvurur. Kimlik bilgilerinin yanında şu girdileri action adımının `with:` bölümüne ekleyebilirsiniz:

| Girdi | Varsayılan | Açıklama |
| --- | --- | --- |
| `effort` | `''` | `ocr review --effort` için `low`, `medium` veya `high`; harf büyüklüğüne duyarsızdır. Boş değer CLI varsayılanını (kayıtlı değer veya medium) korur. OCR v1.10.0+ gerekir; eski sürümde açık hatayla erken durur. |
| `max_tokens_budget` | `''` | `ocr review --max-tokens-budget` için toplam girdi+çıktı token sınırı. Boş veya `'0'` sınırsızdır. Her LLM turundan önce denetlenir. Sınırı aşmış alt göreve bulguları göndermesi için son bir tur verilir; yeni görev dağıtılmaz. Bütçeyi aşan ve atlanan dosyalar `failed(budget)` olur. Yalnızca bütçe nedeniyle kısmi kalmış, kullanılabilir kapsamı olan inceleme 0 ile çıkar. Zaman aşımı/sağlayıcı hatası gibi işletim hataları ve bütçe kaynaklı olsa bile tamamen başarısız incelemeler sıfırdan farklı çıkar. CLI mevcut sonuçları üretir. Sıfırdan farklı çıkışta Action ham çıktıyı artifact olarak yükleyebilir, ancak PR yorumu göndermez. |
| `llm_reasoning_effort` | `''` | `reasoning_effort` alanı ayarlanabilen modeller (ör. GLM-5.x, OpenAI akıl yürütme modelleri) için `minimal`, `low`, `medium`, `high`, `max`; harf büyüklüğüne duyarsızdır. `llm_extra_body` üzerinden gövdeye eklenir ve yayımlanmış tüm CLI sürümleriyle çalışır. `llm_extra_body` içindeki açık `reasoning_effort` önceliklidir. Boşsa gönderilmez. Yalnızca OpenAI uyumlu protokollerde kullanılabilir; Anthropic bilinmeyen gövde alanlarını reddettiğinden action bu protokolde erken durur. Anthropic düşünmesini açık `llm_extra_body` alanıyla yönetin. |
| `stream_progress` | `'false'` | `'true'`, tamamlanana kadar sessiz beklemek yerine canlı `[ocr]` ilerleme satırlarını iş akışı günlüğüne aktarır (insan hedef kitlesi, stderr). Yalnızca görünümü etkiler; artifact ve yorum gönderimi için stderr dosyada da tutulur. |

```yaml
- uses: alibaba/open-code-review@main
  with:
    llm_url: ${{ secrets.OCR_LLM_URL }}
    llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
    llm_model: ${{ vars.OCR_LLM_MODEL }}
    llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
    effort: high
    max_tokens_budget: '10000000'
    llm_reasoning_effort: low
    stream_progress: 'true'
```

Gönderim kipleri (`sticky_summary`, `incremental`), önem derecesi/kategori yönlendirmesi ve push'lar arası kontrol noktaları dahil tam liste için [`action.yml`](https://github.com/alibaba/open-code-review/blob/main/action.yml) dosyasına bakın.

### Özelleştirme {#customization}

Aşağıdaki değişiklikler kopyaladığınız `.github/workflows/ocr-review.yml` dosyasına uygulanır.

#### Arka plan bağlamı {#background-context}

`--background` incelemenin yönlendirilmesinde özellikle etkilidir; [komut satırı ipuçlarına](../../cli-reference/#tips-gotchas) bakın. PR başlığını verin. `feat(auth): add OAuth2 support` gibi anlamlı bir adlandırma düzeni kullanıldığında daha da yararlıdır:

```yaml
- name: Run OCR review
  env:
    PR_TITLE: ${{ github.event.pull_request.title }}
    BASE_REF: ${{ github.base_ref }}
    HEAD_REF: ${{ github.head_ref }}
  run: |
    ocr review \
      --background "$PR_TITLE" \
      --from "origin/$BASE_REF" \
      --to "origin/$HEAD_REF" \
      --format json --audience agent
```

PR tarafından kontrol edilen değerleri doğrudan `run:` içine `${{ }}` ile yerleştirmek yerine `env:` üzerinden geçirin. GitHub, kabuk satırı ayrıştırmadan önce `${{ }}` metnini değiştirir; başlık veya dal adındaki kabuk özel karakterleri aksi halde çalıştırıcıda komut yürütülmesine yol açabilir.

#### Özel kurallar {#custom-rules}

Projeye özgü dosyayı `--rule` ile verin:

```yaml
- name: Run OCR review
  env:
    BASE_REF: ${{ github.base_ref }}
    HEAD_REF: ${{ github.head_ref }}
  run: |
    ocr review --rule ./my-rules.json \
      --from "origin/$BASE_REF" \
      --to "origin/$HEAD_REF"
```

Şema için [İnceleme kuralları](../../review-rules/) belgesine bakın.

#### Eşzamanlılık {#concurrency}

Varsayılan, alt görev başına bir tane olmak üzere 8 paralel alt ajandır. Büyük PR'larda sağlayıcının hız sınırlarını aşmamak için azaltın:

```yaml
- name: Run OCR review
  env:
    BASE_REF: ${{ github.base_ref }}
    HEAD_REF: ${{ github.head_ref }}
  run: |
    ocr review --concurrency 5 \
      --from "origin/$BASE_REF" \
      --to "origin/$HEAD_REF"
```

#### Tetikleme düzeni {#trigger-pattern}

Varsayılan akış PR **açıldığında** ve `/open-code-review` veya `@open-code-review` ile başlayan PR yorumlarında çalışır. İki yaygın değişiklik:

Yeni commit'lerde yeniden inceleme gibi daha fazla yaşam döngüsü olayında çalıştırma:

```yaml
on:
  pull_request:
    types: [opened, synchronize, reopened, ready_for_review]
```

Farklı yorum anahtar sözcüğü kullanma:

```yaml
if: |
  github.event_name == 'pull_request' ||
  (github.event_name == 'issue_comment'
    && github.event.issue.pull_request
    && startsWith(github.event.comment.body, '/review'))
```

`github.event.issue.pull_request`, yorumun sıradan bir issue yerine PR üzerinde olduğunu doğrular.

#### OCR sürümünü sabitleme {#pin-the-ocr-version}

Varsayılan akış son yayımlanan sürümü yükler. Sabitlemek için:

```yaml
- name: Install OpenCodeReview
  run: npm install -g @alibaba-group/open-code-review@1.0.0
```

#### GitHub App kimliğiyle gönderme {#post-under-a-github-app-identity}

Varsayılan yorum sahibi `github-actions[bot]` olur. `OpenCodeReview Bot` gibi adlandırılmış bot için `GITHUB_TOKEN` yerine GitHub App kurulum token'ı kullanın.

1. **Uygulamayı oluşturun:** *Settings → Developer settings → GitHub Apps → New GitHub App*. Bu kullanımda gerekmediğinden webhook'u kapatın. *Repository permissions* altında şu izinleri verin:
   - **Pull requests:** Okuma ve yazma.
   - **Contents:** Salt okunur; farkları almak için.
   - **Metadata:** Salt okunur; zorunlu.
2. Uygulama ayarlarında **özel anahtar üretin**, `.pem` dosyasını indirin ve **App ID** değerini not alın.
3. **Uygulamayı** incelenecek depolara **yükleyin**. Kurulum kimliği işlem sonrası URL'de görünür: `https://github.com/settings/installations/12345` için `12345`.
4. *Settings → Secrets and variables → Actions* altında **üç sır ekleyin**:

| Sır | Değer |
| --- | --- |
| `GITHUB_APP_ID` | Uygulama kimliği. |
| `GITHUB_APP_PRIVATE_KEY` | `-----BEGIN RSA PRIVATE KEY-----` ve `-----END RSA PRIVATE KEY-----` satırları dahil `.pem` dosyasının tamamı. |
| `GITHUB_APP_INSTALLATION_ID` | Kurulum kimliği. |

5. **Token üretip** yorum gönderiminde **kullanın**:

```yaml
   - name: Get GitHub App Token
     id: app-token
     uses: actions/create-github-app-token@v1
     with:
       app-id: ${{ secrets.GITHUB_APP_ID }}
       private-key: ${{ secrets.GITHUB_APP_PRIVATE_KEY }}

   - name: Post review comments to PR
     uses: actions/github-script@v7
     with:
       github-token: ${{ steps.app-token.outputs.token }}
       script: |
         # ...existing post script...
   ```

İncelemeler artık `github-actions[bot]` yerine uygulamanızın adıyla görünür.

#### Bulguları GitHub Code Scanning'e yükleme (SARIF) {#upload-findings-to-github-code-scanning-sarif}

`--format sarif`, stdout'a [SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html) raporu yazar. Dosyaya yönlendirip CodeQL `upload-sarif` action'ıyla yüklediğinizde bulgular **Security → Code scanning** altında görünür:

```yaml
- name: Run OCR review
  env:
    BASE_REF: ${{ github.base_ref }}
    HEAD_REF: ${{ github.head_ref }}
  run: |
    ocr review \
      --from "origin/$BASE_REF" \
      --to "origin/$HEAD_REF" \
      --format sarif --audience agent > results.sarif

- uses: github/codeql-action/upload-sarif@v3
  with:
    sarif_file: results.sarif
```

SARIF makine tarafından okunabilir olduğundan stdout ilerleme satırları bastırılır; `results.sarif` yalnızca raporu içerir. `--preview`, `--format sarif` desteklemez; rapor için tam inceleme veya `ocr scan` çalıştırın.

### Sorun giderme {#troubleshooting}

| Belirti | Neden / çözüm |
| --- | --- |
| `Cannot find merge-base` | Checkout sığ klon kullanmıştır; aralık incelemesi tam geçmiş ister. Kaynak akışın `actions/checkout` adımındaki `fetch-depth: 0` ayarını koruyun. |
| `Failed to parse OCR output` | `OCR_LLM_URL` veya `OCR_LLM_AUTH_TOKEN` eksik/yanlıştır. *Settings → Secrets and variables → Actions* değerlerini kontrol edin. |
| Yorumlar yanlış satırlarda | Genellikle inceleme başlangıcıyla gönderim arasında fark değişmiştir. Betik düz issue yorumuna döner; işlem gerekmez. |

> **Not:** `OCR_DEBUG` ortam değişkeni şu anda uygulanmamıştır; `OCR_DEBUG: "1"` etkisizdir. Gelecekte bağlanması olasılığı için burada anılır. Ayrıntı için akışın `/tmp/ocr-result.json` ve `/tmp/ocr-stderr.log` dosyalarına yazdığı ham JSON ve stderr'i inceleyin veya `ocr review` komutunu yerelde çalıştırın.

## GitLab CI {#gitlab-ci}

Kaynak akış: [`examples/gitlab_ci/.gitlab-ci.yml`](https://github.com/alibaba/open-code-review/blob/main/examples/gitlab_ci/.gitlab-ci.yml).

### Ne yapar? {#what-it-does-2}

- `merge_requests` olaylarında; oluşturma, güncelleme ve yeniden açmada çalışır.
- `node:20` imajında OCR'ı yükler, `ocr config set` ile yapılandırır ve MR fark kipinde çalıştırır.
- Gömülü Python betiğiyle JSON'u ayrıştırır, bulguları fark üzerinde GitLab Discussion olarak gönderir. Doğru konum için MR'ın `versions` uç noktasından `base_sha` / `start_sha` / `head_sha` alır. Satır içine gönderilemeyenler normal MR notu olur; sonunda özet eklenir.

### Kurulum {#install-2}

Akışı depo köküne ekleyin:

```bash
curl -o .gitlab-ci.yml \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/examples/gitlab_ci/.gitlab-ci.yml
```

Mevcut `.gitlab-ci.yml` dosyanızı korumak için örneği başka yola kaydedip `include:` ile ekleyin:

```yaml
include:
  - local: 'ci/ocr-review.gitlab-ci.yml'
```

### Gerekli CI/CD değişkenleri {#required-ci-cd-variables}

**Settings → CI/CD → Variables** altında ayarlayın:

| Değişken | Zorunlu | Maskeli | Açıklama |
| --- | --- | --- | --- |
| `OCR_LLM_URL` | Evet | Hayır | LLM API uç noktası URL'si. |
| `OCR_LLM_AUTH_TOKEN` | Evet | Evet | API token'ı; `ocr config set llm.auth_token` komutuna verilir. OCR'ın doğrudan ortam değişkeni `OCR_LLM_AUTH_TOKEN` değil `OCR_LLM_TOKEN` olur. |
| `OCR_LLM_MODEL` | Hayır | Hayır | Model adı. Varsayılan yoktur; açıkça ayarlanmalıdır. |
| `GITLAB_API_TOKEN` | Hayır | Evet | `api` kapsamlı proje/kişisel/grup erişim token'ı. Yoksa (ör. fork MR'larında) `CI_JOB_TOKEN` kullanılır. Güvenilirlik için özel `GITLAB_API_TOKEN` önerilir. |

> GitLab, 8 karakterden kısa değişkenleri reddettiğinden akışta `llm.use_anthropic` sabit `false` olarak yazılmıştır. Anthropic Claude için betiği doğrudan düzenleyin.

> Akış başlangıçta `ocr config set llm.extra_body '{"thinking": {"type": "disabled"}}'` çalıştırarak düşünme kipini kapatır. Sağlayıcınız açık kalmasını gerektiriyorsa satırı kaldırın.

> **Bot adlandırma ipucu:** Project Access Token ve Group Access Token için MR tartışmalarında görünen ad token'ın **adıdır**. `OpenCodeReview Bot` adı, ek kurulum olmadan adlandırılmış inceleyici sağlar. Daha kalıcı servis hesabı gerekmiyorsa kullanışlıdır; bkz. [Servis hesabı kimliğiyle gönderme](#post-under-a-service-account-identity).

### Özelleştirme {#customization-2}

Aşağıdaki değişiklikler kopyaladığınız `.gitlab-ci.yml` dosyasına uygulanır.

#### Arka plan bağlamı {#background-context-2}

MR başlığını `--background` ile verin. `feat(auth): add OAuth2 support` gibi anlamlı başlıklarda özellikle yararlıdır:

```yaml
script:
  - |
    ocr review \
      --background "$CI_MERGE_REQUEST_TITLE" \
      --from "origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" \
      --to "${CI_COMMIT_SHA}" \
      --format json --audience agent
```

#### Özel kurallar ve eşzamanlılık {#custom-rules-and-concurrency}

GitHub Actions ile aynı seçenekleri kullanın: projeye özgü dosya için `--rule`, paralel alt ajanları sınırlamak için `--concurrency`. Varsayılan, alt görev başına bir ajanla 8'dir:

```yaml
script:
  - |
    ocr review --rule ./my-rules.json --concurrency 5 \
      --from "origin/$CI_MERGE_REQUEST_TARGET_BRANCH_NAME" \
      --to "${CI_COMMIT_SHA}"
```

Şema için [İnceleme kuralları](../../review-rules/) belgesine bakın.

#### OCR sürümünü sabitleme {#pin-the-ocr-version-2}

```yaml
script:
  - npm install -g @alibaba-group/open-code-review@1.0.0
```

#### Her push'ta yeniden incelemeyi önleme {#avoid-re-reviewing-on-every-push}

`only: [merge_requests]`, **her** MR güncellemesinde çalışır; uzun süre açık kalan MR'larda çok token tüketebilir. GitLab'da yerleşik “yalnızca oluşturulunca” olayı yoktur. Önerilen yöntem, inceleme öncesi mevcut OCR notlarını arayıp varsa çıkmaktır. `ocr review` çağrısını Python sarmalayıcısıyla değiştirin:

```python
import json, os, sys, urllib.request

GITLAB_URL = os.environ.get("CI_SERVER_URL", "https://gitlab.com")
PROJECT_ID = os.environ["CI_PROJECT_ID"]
MR_IID     = os.environ["CI_MERGE_REQUEST_IID"]
API_TOKEN  = os.environ["GITLAB_API_TOKEN"]

url = (
    f"{GITLAB_URL}/api/v4/projects/{PROJECT_ID}"
    f"/merge_requests/{MR_IID}/notes?per_page=100"
)
req = urllib.request.Request(url, headers={"PRIVATE-TOKEN": API_TOKEN})
with urllib.request.urlopen(req) as resp:
    notes = json.loads(resp.read().decode())

if any("OpenCodeReview" in n.get("body", "") for n in notes):
    print("OCR already reviewed this MR. Skipping to save tokens.")
    sys.exit(0)

# ...otherwise call `ocr review ...` as usual and write the JSON to
# the file the posting step expects.
```

Yeniden incelemeyi zorlamak için önceki OCR notlarını silin. Sonraki akış bunları bulamayacağı için devam eder.

#### Kendi barındırdığınız GitLab {#self-hosted-gitlab}

Kod değişikliği gerekmez. Gönderim betiği GitLab'ın her çalıştırıcıda otomatik ayarladığı `CI_SERVER_URL` değerini okur. `GITLAB_API_TOKEN` değerinin `gitlab.com` yerine kendi sunucunuzdan verildiğinden emin olun.

#### Servis hesabı kimliğiyle gönderme {#post-under-a-service-account-identity}

Varsayılan olarak tartışmalar `GITLAB_API_TOKEN` sahibinin adıyla görünür. `OpenCodeReview Bot` gibi kimlik için proje kapsamlı servis hesabı kullanın.

1. *Project → Settings → Service Accounts → New service account* üzerinden **hesabı oluşturun**. Seçtiğiniz ad MR tartışmalarında görünür.
2. *Settings → Members → Invite member* üzerinden **projeye davet edin**. Hesabı bulup `Developer` veya `Maintainer` rolü verin; ikisi de tartışma gönderebilir.
3. *Settings → Service Accounts → (hesap) → Add new token* üzerinden **`api` kapsamlı token üretin**. Yalnızca bir kez gösterildiğinden hemen kopyalayın.
4. *Settings → CI/CD → Variables* altında mevcut **`GITLAB_API_TOKEN` değerini değiştirin**; değişken adını aynı tutun.

Tartışmalar artık eski token sahibinin değil servis hesabının adıyla gönderilir.

### Sorun giderme {#troubleshooting-2}

| Belirti | Neden / çözüm |
| --- | --- |
| `Cannot find merge-base` | Sığ klon kullanılmıştır. Tam klon sağlayan `GIT_DEPTH: 0` ayarını koruyun. |
| Gönderimde `API error 403` | Token'da `api` kapsamı yoktur, sahibi proje üyesi değildir veya başka GitLab sunucusundan verilmiştir. `api` kapsamıyla yeniden üretip *Settings → CI/CD → Variables* altına ekleyin. |
| `Failed to parse OCR output` | `OCR_LLM_URL` veya `OCR_LLM_AUTH_TOKEN` yanlıştır; CI/CD değişkenlerini kontrol edin. |
| Satır içi yorum yanlış yerde | GitLab kesin SHA eşleşmesi ister. Betik doğru `base_sha` / `start_sha` / `head_sha` için `versions` üst verisini alır. Konumlandırma yine başarısızsa düz MR notuna döner. |

Akış ham JSON'u `/tmp/ocr-result.json`, stderr'i `/tmp/ocr-stderr.log` dosyasına yazar. Tanılama adımında inceleyin:

```yaml
script:
  - cat /tmp/ocr-result.json
  - cat /tmp/ocr-stderr.log
```

## Ayrıca bkz. {#see-also}

- [Komut satırı başvurusu](../../cli-reference/#json) — İki akışın kullandığı JSON yapısı; kendi CI betiğinizi yazarken yararlıdır.
- [Yapılandırma](../../configuration/) — OCR'ın kullandığı tüm ortam değişkenleri ve yapılandırma anahtarları.

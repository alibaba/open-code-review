---
title: Yapılandırma
sidebar:
  order: 5
---

Yapılandırma dosyası `~/.opencodereview/config.json` konumundadır. Üç şekilde düzenleyebilirsiniz:

- **Etkileşimli terminal arayüzü** — Yönlendirmeli menülerle `ocr config provider` / `ocr config model`.
- **Komut satırı** — Betikler ve CI için uygun olan `ocr config set <key> <value>`.
- **Elle düzenleme (önerilmez)** — JSON dosyasını doğrudan düzenleme. Sonraki `ocr config set` yazma işleminde yeniden biçimlendirilir.

## Model yapılandırma {#configuring-a-model}

### Önerilen: etkileşimli kurulum {#recommended-interactive-setup}

```bash
ocr config provider
```

Yerleşik veya özel sağlayıcı seçmenizi, API anahtarı girmenizi ve model belirlemenizi sağlar. Ayarları dosyaya kaydeder, ardından uç noktayı doğrulamak için bir kez `ocr llm test` çalıştırır. Sonradan model değiştirmek için:

```bash
ocr config model
```

### Etkileşimsiz kurulum (CI / terminal arayüzü olmayan ortamlar) {#non-interactive-setup-ci-no-tui-environments}

Aynı yapılandırmaya `ocr config set` ile yazın:

```bash
ocr config set provider                    anthropic
ocr config set model                       claude-opus-4-6
ocr config set providers.anthropic.api_key sk-ant-xxxxxxxxxx
```

### Yerleşik sağlayıcılar {#built-in-providers}

Aşağıdaki sağlayıcılar, temel URL ve protokol önceden ayarlanmış olarak gelir. Çoğu API anahtarı gerektirir. `providers.<name>.api_key` ayarlanmamışsa ilgili ortam değişkeni kullanılır. `chatgpt` sağlayıcısı ise `ocr auth login` ile oluşturulan ChatGPT kimlik bilgilerini kullanır.

| Ad | Protokol | Temel URL | API anahtarı ortam değişkeni |
|---|---|---|---|
| `anthropic` | anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| `bedrock` | anthropic-bedrock | `aws_region` değerinden türetilir | — (AWS kimlik bilgisi zinciri) |
| `chatgpt` | openai-responses | `https://api.openai.com/v1` | uygulanmaz (ChatGPT ile giriş) |
| `openai` | openai | `https://api.openai.com/v1` | `OPENAI_API_KEY` |
| `openai-responses` | openai-responses | `https://api.openai.com/v1` | `OPENAI_RESPONSES_API_KEY` |
| `openrouter` | openai | `https://openrouter.ai/api/v1` | `OPENROUTER_API_KEY` |
| `gemini` | openai | `https://generativelanguage.googleapis.com/v1beta/openai` | `GEMINI_API_KEY` |
| `dashscope` | openai | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_API_KEY` |
| `dashscope-tokenplan` | openai | `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_TOKENPLAN_KEY` |
| `volcengine` | openai | `https://ark.cn-beijing.volces.com/api/v3` | `ARK_API_KEY` |
| `deepseek` | openai | `https://api.deepseek.com` | `DEEPSEEK_API_KEY` |
| `tencent-tokenhub` | openai | `https://tokenhub.tencentmaas.com/v1` | `TENCENT_TOKENHUB_API_KEY` |
| `hy-tokenplan` | openai | `https://api.lkeap.cloud.tencent.com/plan/v3` | `TENCENT_HUNYUAN_TOKENPLAN_KEY` |
| `iflytek` | openai | `https://spark-api-open.xf-yun.com/v1` | `SPARK_API_KEY` |
| `kimi` | openai | `https://api.moonshot.cn/v1` | `MOONSHOT_API_KEY` |
| `kimi-global` | openai | `https://api.moonshot.ai/v1` | `MOONSHOT_GLOBAL_API_KEY` |
| `z-ai` | openai | `https://open.bigmodel.cn/api/paas/v4` | `Z_AI_API_KEY` |
| `mimo` | openai | `https://api.xiaomimimo.com/v1` | `MIMO_API_KEY` |
| `minimax` | openai | `https://api.minimax.io/v1` | `MINIMAX_GLOBAL_API_KEY` |
| `minimax-cn` | openai | `https://api.minimaxi.com/v1` | `MINIMAX_API_KEY` |
| `baidu-qianfan` | openai | `https://qianfan.baidubce.com/v2` | `QIANFAN_API_KEY` |
| `siliconflow`  | openai | `https://api.siliconflow.com/v1` | `SILICONFLOW_GLOBAL_API_KEY` |
| `siliconflow-cn`  | openai | `https://api.siliconflow.cn/v1` | `SILICONFLOW_API_KEY` |
| `novita` | openai | `https://api.novita.ai/openai` | `NOVITA_API_KEY` |
| `xai` | openai | `https://api.x.ai/v1` | `XAI_API_KEY` |

Yerleşik sağlayıcıların model listeleri, `ocr config model` için öneridir; `--model` seçimini sınırlamaz. Hem yerleşik listede hem `providers.<name>.models` içinde bulunmayan bir model seçilirse stderr'e uyarı yazılır; istek gönderildiğinde modeli sağlayıcı doğrular. Özel sağlayıcıların mevcut `--model` doğrulama kuralları değişmez.

`minimax` ve `minimax-cn` için yeni seçimlerde önce `MiniMax-M3.1-Flash`, ardından `MiniMax-M3` önerilir. Kaydedilmiş modeller ve açıkça verilen eski veya özel model kimlikleri kabul edilmeye devam eder. Ağ geçidi ön ayarları kendi kimliklerini korur; doğrudan MiniMax model kimliğinin bulunması, aynı modelin ağ geçidinde kullanılabildiği anlamına gelmez.

Varsayılan MiniMax isteği, [MiniMax API varsayılanlarının](https://platform.minimax.io/docs/api-reference/text-openai-api) uygulanması için `thinking` ve `reasoning_effort` alanlarını göndermez. Flash, uyarlamalı düşünme gerektirir ve varsayılan çaba düzeyi `max` olur. `extra_body` ile `thinking: {"type":"adaptive"}` ve `low`, `medium`, `high`, `xhigh` veya `max` değerli `reasoning_effort` kullanılabilir. M3 ayrıca `thinking: {"type":"disabled"}` destekler. Kaydedilmiş `extra_body` ayarları korunur; Flash'a geçerken varsa `thinking: {"type":"disabled"}` ayarını kaldırın.

## ChatGPT ile giriş önizlemesi {#sign-in-with-chatgpt-preview}

Bu önizleme, herkese açık Responses API için yerleşik `chatgpt` sağlayıcısını ekler. API anahtarı ortam değişkeni yoktur. Giriş, onay ve kullanım kısıtları için [ChatGPT önizleme kılavuzuna](../chatgpt-preview/) bakın.

Sağlayıcıyı ve desteklenen isteğe bağlı istek alanlarını `ocr config set` ile ayarlayın. `ocr llm models` listesinden model seçin veya model kimliğini açıkça belirtin:

```bash
ocr auth login --provider chatgpt
ocr config set provider chatgpt
ocr config set providers.chatgpt.model <model-slug>
ocr config set providers.chatgpt.extra_body '{"reasoning":{"effort":"low"}}'
ocr llm test
```

Sağlayıcı, hesaba özgü katalogda bulunmasa da açıkça belirtilmiş model değerlerini kabul eder. `extra_body` içinde `reasoning`, `text` ve `prompt_cache_key` alanlarını kabul eder; uç nokta, protokol ve kimlik bilgisi geçersiz kılmalarını reddeder.

### Yerleşik sağlayıcının temel URL'sini değiştirme {#overriding-a-built-in-provider-s-base-url}

Her yerleşik sağlayıcının yukarıdaki tabloda gösterilen bir temel URL'si vardır. Farklı bir uç noktaya yönlendirmek için `providers.<name>.url` ayarlayın. Örneğin kendi barındırdığınız LiteLLM ağ geçidi, ön ayardaki `http://localhost:4000/v1` adresinden farklı olabilir.

`chatgpt` önizlemesi istisnadır: herkese açık uç noktası sabittir ve URL değişikliği reddedilir.

```bash
ocr config set provider                   litellm
ocr config set model                      openai/gpt-5.4
ocr config set providers.litellm.api_key  "$LITELLM_API_KEY"
ocr config set providers.litellm.url      https://gateway.internal:8000/v1
```

Yapılandırılan `url`, ön ayardan önceliklidir. Ayar yoksa veya temizlenirse varsayılan adres kullanılır; yalnızca uç noktanız farklıysa ayarlamanız gerekir.

### AWS Bedrock {#aws-bedrock}

Varsayılan `anthropic-bedrock` protokolünde `bedrock`, `anthropic` ile aynı Messages API'yi kullanır. Ancak API anahtarı yerine standart AWS kimlik bilgisi zinciriyle SigV4 imzası oluşturulur; sunucuyu bölge belirler. Ayarlanacak bir `api_key` yoktur ve imza yerine API anahtarı kabul edilmez:

```bash
ocr config set provider                      bedrock
ocr config set model                         us.anthropic.claude-sonnet-4-6
ocr config set providers.bedrock.aws_region  us-west-2
ocr config set providers.bedrock.aws_profile example-profile
```

OpenAI uyumlu ağ geçidi için `providers.bedrock.protocol` değerini `openai` yaparsanız URL'yi `providers.bedrock.url`, API anahtarını `providers.bedrock.api_key` veya `providers.bedrock.api_key_cmd` ile verin. `ocr config provider` içinde başka model seçmek kayıtlı API anahtarını korur.

| Alan | Anlamı |
| --- | --- |
| `providers.bedrock.aws_region` | İsteği karşılayan `bedrock-runtime` sunucusunun bölgesi. Ayarlanmamışsa `AWS_REGION` veya etkin profil kullanılır. |
| `providers.bedrock.aws_profile` | Kimlik bilgilerini sağlayan adlandırılmış profil. Ayarlanmamışsa `AWS_PROFILE` veya mevcut zincir kullanılır. |

İki alan da isteğe bağlıdır. Boş bırakıldığında diğer AWS araçlarında olduğu gibi standart zincir karar verir. Açıkça belirtmek, özellikle varsayılanı farklı CI çalıştırıcılarında, önce `AWS_PROFILE` dışa aktarmadan tekrarlanabilir çalıştırmalar sağlar.

Model kimlikleri hem hesaba **hem bölgeye** bağlıdır. OCR'ın listesi kapalı bir küme değil, başlangıç noktasıdır. Hesabınızda geçerli çıkarım profili kimliği veya uygulama çıkarım profili ARN'si listede olmasa da kabul edilir. Mevcut seçenekleri görmek için `aws bedrock list-inference-profiles --region <region>` çalıştırın. Yeni model ailelerinde `-v1:0` gibi sürüm sonekleri geçersizdir.

Varsayılan `anthropic-bedrock` protokolünde `ocr llm test`, URL yerine bölge ve profili gösterir. Sunucuyu bölge belirler:

```
Source: provider:bedrock
Region: us-east-1
Profile: example-profile
Model:  claude-sonnet-5
✓ Connection test successful
```

Bedrock, `llm.protocol` veya `OCR_LLM_PROTOCOL` üzerinden kullanılamaz. Bu blok tek URL ve token tanımlar; bölge ya da profil alanı içermez. Bedrock bu iki değeri de kullanmadığı için bu birleşim sessizce yok sayılmak yerine reddedilir.

### Özel sağlayıcılar {#custom-providers}

`mimo-v2.6-flash` ve `foo.api_key` gibi adlar desteklenir. Nokta içeren yeni bir adın alanlarını ayarlamadan önce `ocr config set provider <name>` ile oluşturup seçin; bu işlem etkin sağlayıcıyı değiştirir. Mevcut noktalı girdiler seçilmeden `providers.<name>.<field>` veya `custom_providers.<name>.<field>` üzerinden düzenlenebilir. Adlar noktayla başlayıp bitemez, art arda noktalar ya da boş/sadece boşluk içeren bölümler içeremez. Noktasız adlar örtük oluşturmayı desteklemeye devam eder.

Yukarıdaki tabloda bulunmayan sağlayıcı adları özel kabul edilir ve en az `url` ile `protocol` gerektirir. Protokol `anthropic`, `openai`, `openai-responses` veya `anthropic-bedrock` olabilir.

Chat Completions sunan OpenAI uyumlu bir uzak sağlayıcı için `protocol` alanını `openai` olarak ayarlayın. `url` alanına sağlayıcının belgelerinde belirtilen API kök adresini girin; bu adres genellikle `/v1` ile biter. OCR, isteği gönderirken adrese `/chat/completions` ekler. Örneğin OpenAI'ın herkese açık Chat Completions uç noktasına şu adla bağlanabilirsiniz:

```bash
ocr config set provider                                      openai-chat-gateway
ocr config set custom_providers.openai-chat-gateway.url      https://api.openai.com/v1
ocr config set custom_providers.openai-chat-gateway.protocol openai
ocr config set custom_providers.openai-chat-gateway.model    gpt-4.1
ocr config set custom_providers.openai-chat-gateway.api_key  "$OPENAI_API_KEY"
```

Diğer OpenAI uyumlu uzak hizmetler için de aynı alanları kullanın. `url` yerine hizmetin API kök adresini, `model` yerine kataloğunda kabul ettiği model kimliğini yazın. OCR, araçları Chat Completions üzerinden gönderdiğinden hem model hem de uç nokta yerleşik araç çağrılarını desteklemelidir. Özel sağlayıcılarda OCR, API anahtarı için ortam değişkeni adını kendiliğinden belirlemez; `api_key` veya `api_key_cmd` ayarlanmalıdır. `ocr config set provider <name>`, sağlayıcıyı etkinleştirir ve OCR'nin hangi yapılandırmayı kullanacağını değiştirir.

Sağlayıcı veya model OpenAI Responses API (`/v1/responses`) gerektiriyorsa `openai-responses` kullanın:

```bash
ocr config set provider                                               openai-responses-gateway
ocr config set custom_providers.openai-responses-gateway.url          https://api.openai.com/v1
ocr config set custom_providers.openai-responses-gateway.protocol     openai-responses
ocr config set custom_providers.openai-responses-gateway.model        gpt-5
ocr config set custom_providers.openai-responses-gateway.api_key      "$OPENAI_API_KEY"
```

`anthropic-bedrock` protokolündeki özel sağlayıcıda `url` gerekmez; sunucuyu bölge belirler. Yerleşik sağlayıcıyla aynı AWS alanlarını kabul eder. İkinci bölge veya profil için ayrı girdi şöyle oluşturulur:

```bash
ocr config set provider                                bedrock-eu
ocr config set custom_providers.bedrock-eu.protocol    anthropic-bedrock
ocr config set custom_providers.bedrock-eu.aws_region  eu-west-1
ocr config set custom_providers.bedrock-eu.aws_profile eu-profile
ocr config set custom_providers.bedrock-eu.model       eu.anthropic.claude-sonnet-4-6
```

`url`, API'nin temel adresi veya tam `/responses` uç noktası olabilir; OCR ikisini de normalleştirir.

Ollama ile sunulan yerel model, yerel OpenAI uyumlu uç noktayı gösteren özel sağlayıcı olarak tanımlanır:

```bash
ocr config set provider                          ollama
ocr config set custom_providers.ollama.url       http://127.0.0.1:11434/v1
ocr config set custom_providers.ollama.protocol  openai
ocr config set custom_providers.ollama.model     qwen3:32b
ocr config set custom_providers.ollama.api_key   ollama
```

Ollama API anahtarını kullanmaz; ancak özel sağlayıcılar boş olmayan `api_key` gerektirir ve ortam değişkenine başvurmaz. Bu nedenle herhangi bir yer tutucu değer kullanın. Modelin yerel araç çağrısını desteklemesi gerekir. Seçmeden önce SSS'deki [“No tool calls parsed” (yerel modeller / Ollama)](../faq/#no-tool-calls-parsed-local-models-ollama) bölümünü okuyun.

### Zaman aşımları {#timeouts}

Görev ve istek zaman aşımları birbirinden bağımsızdır:

- `ocr review --timeout` ve `ocr scan --timeout`, eşzamanlı görev bütçesini **dakika** olarak belirler. Varsayılan **15**; `0` görev süresi sınırını kaldırır. Bütçe tüm LLM çağrılarını, araç işlemlerini ve yeniden deneme beklemelerini kapsar. İnceleme, bunu çaba turu sayısına göre ölçekler.
- İstek başına HTTP zaman aşımı **saniye** cinsindendir; varsayılan **300**. `--timeout` artırıldığında değişmez. Görev sınırını kaldırmak istek sınırını kaldırmaz. Kalan görev süresi daha kısaysa o geçerlidir.

Yavaş yerel modeller veya büyük dosyalar 300 saniyeden fazlasını gerektirebilir. Kapsamı giderek genişleyen üç ayar vardır:

- `providers.<name>.timeout_sec` / `custom_providers.<name>.timeout_sec` — Sağlayıcıya özgü saniye değeri.
- `llm.timeout_sec` — Eski `llm` bölümü için saniye değeri.
- `OCR_LLM_TIMEOUT` — Tam sayı saniye alan ortam değişkeni; tüm çözümleme yollarında yapılandırma dosyasından önceliklidir.

İki `timeout_sec` alanı da `ocr config set` ile ayarlanabilir:

```json
{
  "custom_providers": {
    "ollama": { "url": "http://127.0.0.1:11434/v1", "protocol": "openai", "timeout_sec": 900 }
  }
}
```

Örneğin `OCR_LLM_TIMEOUT=900 ocr review --timeout 30`, görev bütçesi içinde tek bir isteğe 15 dakika tanır. Ortam değişkeni veya `timeout_sec` ayarı yoksa `--timeout 30` verilse bile istek 5 dakikada zaman aşımına uğrar.

İstek başına süre dolduğunda, yanıt gövdesi veya akış okunurken de dahil olmak üzere çağrı otomatik tekrarlanmadan sona erer. Sürekli yavaş isteklerde aynı sınırla tekrar denemek yerine istek zaman aşımını artırın. SDK'nın mevcut yeniden deneme ilkesi değişmez: yeniden denenebilir bağlantı ve HTTP hataları, artan bekleme süreleri ve sağlayıcının `Retry-After` yönlendirmeleriyle en fazla 5 kez yeniden denenir. SDK'nın deneme başına süresi, beklemeyi de sınırlar. İptal veya görev süresinin dolması hem istekleri hem beklemeleri durdurur.

Tanılama mesajları, `LLM request timeout` (kontrol edilecek ayarlar: `OCR_LLM_TIMEOUT` veya sağlayıcı `timeout_sec`) ile çağıran işlemin bağlamının süresinin dolduğunu belirten `caller deadline exceeded` durumlarını ayırır. İnceleme/tarama görevi için `--timeout` ayarına bakın. Arka plan bellek sıkıştırması ve `ocr llm test` gibi işlemlerin kendi süreleri vardır.

### API anahtarını komuttan alma {#api-key-from-a-command}

Anahtarı dosyada tutmak yerine `api_key_cmd` ile çalışma anında bir gizli bilgi yöneticisinden (1Password, `pass`, `gopass` vb.) alın. Baştaki/sondaki boşlukları temizlenmiş tek satırlık stdout çıktısı anahtar olur. Eski `llm` bloğundaki karşılığı `auth_token_cmd` alanıdır.

```bash
ocr config set providers.anthropic.api_key_cmd "op read op://dev/anthropic/api-key"
```

İşletim sisteminin anahtarlığı da kendi aracı üzerinden kullanılabilir; anahtar `config.json` yerine Keychain veya Secret Service içinde kalır:

```bash
# macOS Keychain
ocr config set providers.anthropic.api_key_cmd \
  "security find-generic-password -s ocr-anthropic -w"

# Linux (Secret Service: GNOME Keyring, KWallet, …)
ocr config set providers.anthropic.api_key_cmd \
  "secret-tool lookup service ocr-anthropic"
```

Öncelik: sabit `api_key` her zaman kazanır. İkisi de ayarlanmışsa komut yok sayılır ve uyarı yazılır. Aksi halde `api_key_cmd` çalışır. Yalnızca ikisi de ayarlanmamışsa sağlayıcının ortam değişkenine başvurulur.

Komut, her `ocr` çalıştırmasında bir kez yürütülür ve başarılı olmak zorundadır. Sıfırdan farklı çıkış, boş/çok satırlı çıktı veya 64 KiB üzeri çıktı kesin hatadır; OCR sessizce başka kaynağa geçmez. Komut, istemlere yanıt vermek için geçen süre dahil 60 saniyede tamamlanmalıdır. Terminalin stdin ve stderr akışlarını devraldığı için pinentry veya Touch ID gibi etkileşimli istemler görünür ve yanıtlanabilir. Arka plan servisi (`gpg-agent`, ilk kullanımdaki `op` servisi) stdout borusunu açık tutarsa anahtar alınır ancak her çalıştırma borunun kapanması için fazladan 5 saniye bekler. Bu beklemeyi kaldırmak için servisin çıktısını `>/dev/null 2>&1` ile yönlendirin.

Windows'ta komut `sh` değil `cmd.exe` üzerinden yürütülür; birine yazılmış komut genellikle diğerine taşınabilir değildir. `%VAR%` ve `^`, `cmd.exe` özel karakterleridir; `$VAR` açılımı ve `\` kaçışlaması burada uygulanmaz. Tırnak içindeki argümanlar aynen geçirilir; `op read "op://Private/My Vault/api-key"` yazıldığı biçimiyle çalışır.

Değer kabuk komutu olarak yürütüldüğünden `config.json` güvenilen girdidir. Dosyanın sahibi siz olun ve başkalarına yazma izni vermeyin; OCR dosyayı `0600` izinleriyle yazar.

### Ek yeniden deneme durum kodları {#additional-retry-status-codes}

Bazı sağlayıcılar geçici hatalar için standart dışı 4xx kodları kullanır; örneğin hız sınırında `403` veya `400` döndürebilir. Mevcut SDK mekanizmasıyla bu istekleri yeniden denemek için `retry_codes` kullanın.

`retry_codes`, tam sayı dizisidir. `llm.retry_codes` veya `custom_providers.<name>.retry_codes` olarak ayarlanabilir. `ocr config set` ile virgülle ayrılmış liste verin:

```bash
ocr config set llm.retry_codes 403,400
ocr config set custom_providers.my-gateway.retry_codes 403,400
```

Yalnızca 4xx HTTP kodları kabul edilir. `408`, `409` ve `429` zaten SDK tarafından yeniden denenir; dosyadan okunurken bu gereksiz kodlar yok sayılır. `ocr config set` ayrıca uyarır ve bunları kaydedilen değerden çıkarır. Tüm 5xx yanıtları zaten yeniden denendiğinden `retry_codes` içine eklenemez. Ancak `429` veya `retry_codes` yanıtları kalıcı bütçe ya da kota tükenmesi hataları (`budget_exceeded`, `insufficient_quota`) içeriyorsa SDK yeniden denemeleri atlanır ve işlem hemen durdurulur; geçici hız sınırı `429` yanıtları mevcut yeniden deneme davranışını korur.

### İstem sınırı {#prompt-limit}

`max_tokens`, tek alt görev (bir dosya veya ilişkili dosyalar grubu) için **istem/girdi** sınırıdır. Gömülü şablonlarda `ocr review` için 200.000, `ocr scan` için 58.888 token varsayılır. Farklı bağlam pencereli model için ayarı kaydedin:

```bash
ocr config set max_tokens 400000
```

Ayar hem `ocr review` hem `ocr scan` için geçerlidir. Kayıtlı yapılandırmayı değiştirmeden tek çalıştırmada geçersiz kılmak için `--max-tokens` kullanın:

```bash
ocr review --max-tokens 400000
ocr scan --max-tokens 400000
```

Komut seçeneği kayıtlı `max_tokens` değerinden önceliklidir. İkisi de yoksa gömülü görev şablonu varsayılanı kullanılır. Bu sınır, modelin **çıktı** sınırından (iki şablonda da `MAX_COMPLETION_TOKENS = 16384`) ve tüm çalıştırmanın toplam token kullanımını sınırlayan `--max-tokens-budget` değerinden bağımsızdır. Gömülü varsayılana dönmek için `ocr config unset max_tokens` kullanın.

### İnceleme çabası {#review-effort}

`effort`, alt görev başına inceleme turu sayısını belirler: `low` = 1, `medium` (varsayılan) = 2, `high` = 3. Daha fazla tur daha fazla sorun bulabilir; maliyeti de orantılı artırır.

```bash
ocr config set effort high
ocr config unset effort      # back to the default medium
```

`--effort low|medium|high`, tek çalıştırmada kayıtlı değeri geçersiz kılar.

### Bağlantıyı doğrulama {#verify-connectivity}

```bash
ocr llm test
```

### Mevcut ortam değişkenlerini kullanma {#reuse-existing-environment-variables}

Claude Code'un `ANTHROPIC_*` veya OCR'ın `OCR_LLM_*` değişkenlerini zaten ayarladıysanız OCR bunları otomatik kullanır; yapılandırma dosyası gerekmez.

### CC-Switch kullanımı {#using-cc-switch}

[CC-Switch](https://github.com/farion1231/cc-switch) uygulamasını [yönlendirme servisi](https://www.ccswitch.io/en/docs?section=proxy&item=service) etkin olarak kullanıyorsanız sağlayıcının `url` alanını yerel vekil sunucuya yönlendirin; başka ayar gerekmez:

```bash
# Claude (Anthropic-compatible)
ocr config set providers.anthropic.url http://127.0.0.1:15721

# Codex / OpenAI-compatible — set that provider's url key instead
ocr config set providers.<name>.url http://127.0.0.1:15721/v1
```

`api_key` herhangi bir değer olabilir. `extra_body` ve sağlayıcıya özgü diğer alanlar normal şekilde uygulanır.

### Sağlayıcıya özgü alanları gönderme {#send-vendor-specific-fields}

Bazı sağlayıcılar standart dışı istek alanları (ör. Bedrock biçiminde `thinking`) gerektirir. Kaynak kodunu değiştirmeden bunları her isteğe eklemek için `extra_body` kullanın:

```bash
ocr config set providers.anthropic.extra_body '{"thinking":{"type":"disabled"}}'
```

### İstem önbelleği için oturum yakınlığı {#session-affinity-for-prompt-caching}

OCR her LLM konuşması için inceleme oturumuna ve içindeki göreve bağlı bir önbellek yakınlık anahtarı türetir: `<session-id>-<task-type>-<scope-hash>`. İstem önbellekleri öneklerle eşleşir. Konuşma başına anahtar, büyüyen her konuşmayı (ör. bir dosyanın araç döngüsünü) aynı önbellek düğümünde tutar; tüm çalıştırmayı tek yoğun anahtara bağlamaz. Oturum kimliği öneki, sağlayıcı önbellek günlükleriyle `ocr session` kayıtlarını ilişkilendirmeyi sağlar.

Etkinleştirmek için sağlayıcının beklediği `extra_headers` veya `extra_body` değerine `{ocr_session_key}` ekleyin. OCR bunu her istekte konuşmanın anahtarıyla değiştirir; ayarlanmamışsa hiçbir şey göndermez:

```bash
# By OpenAI-style request body field (e.g. prompt_cache_key)
ocr config set providers.openai.extra_body '{"prompt_cache_key": "{ocr_session_key}"}'

# By HTTP header (e.g. x-session-affinity)
ocr config set custom_providers.my-gateway.extra_headers "x-session-affinity={ocr_session_key}"
```

## İnceleme dilini ayarlama {#configuring-the-review-language}

`language`, inceleme yorumlarının dilini belirler. Ayarlanmamışsa varsayılan İngilizcedir:

```bash
ocr config set language 中文
ocr config set language English
```

Türkçe yorumlar için `ocr config set language Türkçe` kullanın.

## Ayrıca bkz. {#see-also}

- [Hızlı başlangıç](../quickstart/) — En az ayarla kurulum ve ilk inceleme.
- [Komut satırı başvurusu](../cli-reference/) — İnceleme komutunun tüm seçenekleri.

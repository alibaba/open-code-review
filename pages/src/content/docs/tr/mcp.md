---
title: MCP sunucuları
sidebar:
  order: 10
---

OCR bir **Model Context Protocol (MCP) istemcisi** olarak çalışabilir. Bir veya daha fazla haricî MCP sunucusu tanımladığınızda bu sunucuların sunduğu araçlar, `file_read` ve `code_search` gibi [yerleşik araçların](../tools/) yanında inceleme ajanına açılır.

MCP, özel inceleme ajanını haricî iş bağlamıyla genişletir; yerleşik araç adlarını veya tanımlarını değiştiremez. Şu anda `ocr review` içinde bağlıdır, `ocr scan` içinde değildir. Karşılaştırma sonuçları sınanan ajan yapılandırması için geçerlidir; her MCP sunucusu birleşimi için geçerli değildir.

Salt okunur bağlam araçlarını ve dar bir `tools` izin listesini tercih edin. OCR, MCP araçlarını salt okunur davranmaya zorlamaz; etkinleştirmeden önce her sunucunun izinlerini ve veri erişimini inceleyin.

## Ne zaman kullanılır? {#when-to-use-it}

İnceleme için farkın dışındaki bağlam yararlı olacaksa MCP sunucusu kullanın:

- **İş kaydı / talep arama** — değişikliğin gereksinimi karşılayıp karşılamadığını kontrol etmek için bağlı Jira / GitHub kaydını ajanın almasını sağlayın.
- **Belgeler / bilgi tabanı** — yorumların gerçek kurum kurallarına dayanması için kurum içi API belgelerini veya kodlama standartlarını alın.
- **Özel analiz** — kod denetleyici, şema doğrulayıcı veya bağımlılık kontrolünü inceleyicinin gerektiğinde çağırabileceği araç olarak sunun.

Yalnızca depoyu okumak gerekiyorsa yerleşik araçlar yeterlidir; MCP, yerel depo kopyasının ötesine erişmek içindir.

## Yapılandırma {#configuration}

#### Yerel MCP sunucusu ekleme {#adding-a-local-mcp-server}

`ocr config set` alanları etkileşimsiz yazar. Dizi alanları (`args`, `env`, `tools`) JSON dizisi metni alır:

```bash
# Minimal: just a command
ocr config set mcp_servers.docs.command npx

# Arguments
ocr config set mcp_servers.docs.args '["-y", "@acme/docs-mcp-server"]'

# Restrict which tools are exposed to the reviewer
ocr config set mcp_servers.docs.tools '["search_docs", "get_page"]'

# A setup command to run before the server starts
ocr config set mcp_servers.docs.setup "npm install -g @acme/docs-mcp-server"

# Environment variables (KEY=VALUE entries)
ocr config set mcp_servers.docs.env '["DOCS_TOKEN=secret", "DOCS_REGION=eu"]'
```

#### Uzak MCP sunucusu ekleme {#adding-a-remote-mcp-server}

**Streamable HTTP** destekleyen sunucularda `type` değerini `remote` yapın ve yerel komut yerine `url` belirtin. Yalnızca `url` yeterli değildir; varsayılan tür `stdio` olur.

Mevcut bağlantıyı değiştirmemek için yeni bir sunucu adı kullanın:

```bash
ocr config set mcp_servers.search.type remote
ocr config set mcp_servers.search.url https://mcp.example.com/mcp
ocr config set mcp_servers.search.tools '["search", "fetch"]'
```

Komutlar bağlantıyı kullanıcı yapılandırmasına kaydeder. Sonraki incelemede OCR bağlanır ve `search` ile `fetch` araçlarını yerleşik araçlarla birlikte ajana sunar. İzin listesi, sunucunun sunabileceği diğer araçları incelemenin dışında tutar. Diğer sunucular ve inceleme ayarlarınız değişmez.

Yapılandırmadan sonra ajan bu araçları her çağrı öncesinde sormadan kullanabilir. Araç argümanları (arama sorguları, istenen URL'ler ve ajanın eklediği bağlam) makinenizden çıkarak uç noktanın işletmecisine ulaşır. Kullanıcı yapılandırması olduğu için tüm depolarda geçerlidir. Yalnızca dış isteklere izin verilen yerlerde etkinleştirin; isteklere gizli bilgi, özel kod veya kurum içi URL eklemeyin. Üçüncü taraf hizmete bağlanmadan önce işletmecinin gizlilik politikasını ve koşullarını inceleyin.

#### MCP sunucusu kaldırma {#removing-an-mcp-server}

`unset` ile kaldırın:

```bash
ocr config unset mcp_servers.docs
```

MCP sunucuları kullanıcı yapılandırma dosyasındaki (`~/.opencodereview/config.json`) `mcp_servers` anahtarı altında tutulur.

| Alan | Tür | Zorunlu | Açıklama |
|---|---|---|---|
| `type` | string | | Yerel alt süreç için `stdio` (varsayılan), Streamable HTTP için `remote`. |
| `command` | string | `stdio` için | MCP sunucusunu başlatan çalıştırılabilir dosya (ör. `npx`, `uvx`, mutlak yol). |
| `args` | string array | | `command` komutuna iletilen argümanlar (yalnızca `stdio`). |
| `url` | string | `remote` için | HTTP veya HTTPS MCP uç noktası. |
| `headers` | object | | HTTP başlık adları ve metin değerleri (yalnızca `remote`). Değerlerdeki `$VAR` veya `${VAR}`, bağlantı sırasında OCR ortamından genişletilir. Boş değere genişleyen başlık boş gönderilmek veya atılmak yerine **bağlantıyı başarısız kılar**. Anonim erişimde belirtmeyin. |
| `tools` | string array | | Kaydedilecek araç adlarının izin listesi. Boşsa sunucunun sunduğu tüm araçlar kaydedilir. |
| `setup` | string | | Sunucu başlamadan önce bir kez çalıştırılan kabuk komutu (yalnızca `stdio`; ör. bağımlılık yükleme). Depo kökünde, 5 dakika zaman aşımıyla çalışır. |
| `env` | string array | | `KEY=VALUE` biçiminde ek alt süreç ortam değişkenleri (yalnızca `stdio`). |

Kimlik doğrulama isteyen uzak sunucularda `headers` için sunucunun yönergelerini izleyin. Ortam değişkeni referansları içeren JSON'u `ocr config set` komutuna geçirirken tek tırnak kullanın; böylece kabuk, OCR yapılandırmayı kaydetmeden önce değişkenleri genişletmez. Anonim erişime izin veren sunucularda `headers` gerekmez.

## Araç filtreleme {#filtering-tools}

Varsayılan olarak sunucunun duyurduğu tüm araçlar kaydedilir. Sunucu ihtiyaçtan fazlasını sunuyorsa `tools` izin listesi belirleyin. Az ve amaca uygun araç, ajanın odağını korur ve token maliyetini azaltır. Listede olup sunucunun sunmadığı adlar uyarıyla atlanır; yazım hataları sessizce geçmek yerine stderr üzerinde görünür.

## Ad çakışmaları {#name-conflicts}

MCP araçları yerleşik araçlarla aynı ad alanını kullanır. Bir araç adı **yerleşik/ayrılmış** araçla (`file_read`, `code_search`, `task_done`, …) veya başka MCP sunucusunun önceden kaydettiği araçla çakışırsa OCR aracı **atlar** ve uyarı kaydeder. İlk kayıt geçerlidir; araçların kaybolmaması için benzersiz adlar kullanın.

## `setup` komutu {#the-setup-command}

`setup`, sunucu alt süreci başlamadan önce depo kökünde bir kez çalışır. Sunucuyu gerektiğinde yüklemek veya derlemek için kullanın:

```json
"setup": "npm install -g @acme/docs-mcp-server"
```

**5 dakika zaman aşımı** vardır. Sıfırdan farklı kodla çıkarsa OCR komutu, çalışma dizinini ve çıktıyı kaydeder; o sunucuyu atlayıp incelemeye devam eder.

## Sorun giderme {#troubleshooting}

Tüm MCP tanılama iletileri `[ocr]` önekiyle **stderr** üzerine yazılır; stdout üzerindeki `--format json` çıktısını bozmaz:

- `Running setup for MCP server "x": …` — kurulum komutu çalışıyor.
- `failed to start MCP server "x": …` — alt süreç 30 saniyelik başlangıç zaman aşımında bağlanamadı veya `command`, `PATH` üzerinde değil.
- `remote MCP server "x" has no URL configured, skipping` — `type`, `remote` ancak `url` ayarlanmamış; `type` olmadan `url` ayarlamanın tersi durumdur.
- `failed to connect to remote MCP server "x": …` — uç nokta 30 saniyelik başlangıç zaman aşımında bağlanamadı veya araçlarını listeleyemedi. URL'yi, ağ erişimini ve gerekli başlıkları kontrol edin.
- `MCP server "x" header "h" expanded to empty value` — `headers` içindeki `$VAR`, OCR ortamında ayarlı değil. Bağlantı başarısız olur; başlık yok sayılmaz.
- `remote MCP server "x" returned HTTP 401 Unauthorized` — token veya başlık yapılandırmasını kontrol edin.
- `remote MCP server "x" returned HTTP 403 Forbidden` — kimlik bilgileri sunucuya ulaştı ancak gerekli yetkiler yok.
- `tool "y" conflicts with built-in tool, skipping` — sunucu aracını yeniden adlandırın veya `tools` listesinden çıkarın.
- `allowed tool "y" not found in server's tool list` — `tools` içindeki ad sunucunun sunduğu araçlarla eşleşmiyor; yazımı kontrol edin.

Başlatılamayan veya bağlanamayan sunucu atlanır; inceleme onun araçları olmadan devam eder.

## Ayrıca bakın {#see-also}

- [Araçlar](../tools/) — MCP araçlarının yanında bulunan altı yerleşik araç.
- [Yapılandırma](../configuration/) — tüm yapılandırma dosyası ve anahtarlar.
- [CLI başvurusu](../cli-reference/) — `ocr config` ve inceleme seçenekleri.

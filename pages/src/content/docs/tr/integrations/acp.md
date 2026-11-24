---
title: ACP
sidebar:
  order: 6
---

**Deneysel özellik:** ACP bağdaştırıcısı şu anda kararlı değildir ve yalnızca yerelde kaynaktan derlendiğinde desteklenir. Yükseltirken kaynağı güncelleyip yeniden derleyin.

## Ön koşullar {#prerequisites}

- [OCR CLI](../../installation/)'yi yükleyin, [inceleme modelini](../../configuration/) yapılandırın ve `ocr llm test` ile bağlantıyı kontrol edin.
- Bağdaştırıcıyı derlemek için `acp/` içeren kaynak kopyası, Git ve Go 1.26.10+ hazırlayın. OCR CLI'yi kaynaktan derlemek de Go 1.26.10+ gerektirir.
- ACP v1 stdio ve özel ajan başlatma komutlarını destekleyen istemci kullanın.

Bağdaştırıcı ve OCR CLI ayrı derlenir. Aşağıdaki yönergeler yerel derlemeyi kullanır.

## Derleme ve istemciyi bağlama {#build-and-connect-your-client}

Depo kökünde çalıştırın:

```bash
make -C acp build
command -v ocr
```

Derleme çıktısı `dist/ocr-acp` dosyasıdır. `command -v ocr` çıktısındaki mutlak yolu aşağıdaki `--ocr-binary` değerine yazın.

İstemcinin Agent ayarlarına şu yapılandırmayı ekleyin. Tüm `/absolute/path` yer tutucularını ve ayrıştırıcı model değerlerini kendi ayarlarınızla değiştirin:

```json
{
  "agent_servers": {
    "OpenCodeReview": {
      "type": "custom",
      "command": "/absolute/path/open-code-review/dist/ocr-acp",
      "args": [
        "--ocr-binary",
        "/absolute/path/ocr"
      ],
      "env": {
        "OCR_ACP_PARSER_PROVIDER": "anthropic",
        "OCR_ACP_PARSER_MODEL": "deepseek-flash",
        "OCR_ACP_PARSER_BASE_URL": "https://your-gateway.example",
        "OCR_ACP_PARSER_API_KEY": "YOUR_PARSER_API_KEY"
      }
    }
  }
}
```

Alan adları istemciye göre değişir; istemcinin ACP yapılandırma kılavuzuna bakın.

## İnceleme veya tarama çalıştırma {#run-a-review-or-scan}

OpenCodeReview konuşmasında şu komutları gönderin:

| İstek | Komut |
| --- | --- |
| Hazırlanmış, hazırlanmamış ve izlenmeyen değişiklikleri incele | `/review` |
| En fazla bir inceleme turu çalıştır | `/review --effort low` |
| Çalışma kopyasında seçili son commit'i incele | `/review --commit HEAD` |
| Mevcut iki referansı karşılaştır | `/review --from main --to HEAD` |
| Bir dizini tara | `/scan --path internal/agent` |
| Birden fazla yolu tara | `/scan --path internal/agent,internal/config` |

İnceleme, çalışma dizininin Git deposu olmasını gerektirir ve sonuç yollarını depo kökünden çözümler. Tarama, oturum çalışma dizinini kök olarak kullanır.

`--effort low` en fazla bir, `medium` iki, `high` üç inceleme turu çalıştırır. Bir tur yeni bulgu üretmezse inceleme erken bitebilir. Seçenek verilmezse OCR'nin yapılandırılmış değeri kullanılır. Daha az tur bazı bulguları kaçırabilir; ihtiyacınıza göre seçin.

ACP komut önerilerini destekleyen istemcilerde komut ve seçenekleri görmek için `/` yazın.

İstemci yerel dosya bağlantıları ekliyorsa bunları `/scan` ile gönderin ve `--path` belirtmeyin. Dosya bağlantıları `/review` veya açık tarama yoluyla birleştirilemez. Desteklenmeyen veya belirsiz bağlantılar, kapsam genişletilmek yerine reddedilir.

Bağdaştırıcı yalnızca belgelenmiş CLI seçeneklerini kabul eder:

- İnceleme: `--commit`, `--from`, `--to`, `--effort`, `--no-filter`, `--background`, `--background-file`.
- Tarama: `--path`, `--batch`, `--no-plan`, `--no-dedup`, `--no-summary`, `--background`.

`/review`, `--staged` desteklemez. Konuşma komutları `--format` ve `--audience` gibi çıktı seçeneklerini ve `--repo`, `--model`, `--provider` gibi geçersiz kılma seçeneklerini de reddeder. İnceleme modelini OCR üzerinden yapılandırın.

## Doğal dil isteklerini etkinleştirme {#enable-natural-language-requests}

Ayrıştırıcı model, doğal dil isteklerini inceleme veya tarama komutlarına dönüştürür; eksik bilgi varsa açıklama ister. OCR inceleme modelinden ayrı yapılandırılır. İstemcinin model seçimi bu iki modeli de yapılandırmaz.

Agent'ın `env` nesnesine şu ayarları ekleyip model ve anahtar yer tutucularını değiştirin:

```json
{
  "OCR_ACP_PARSER_PROVIDER": "openai",
  "OCR_ACP_PARSER_MODEL": "YOUR_TOOL_CALLING_MODEL",
  "OCR_ACP_PARSER_BASE_URL": "https://your-gateway.example",
  "OCR_ACP_PARSER_API_KEY": "YOUR_PARSER_API_KEY"
}
```

Bu örnek OpenAI uyumlu Chat Completions protokolünü kullanır. Anthropic Messages için sağlayıcıyı `anthropic` yapın. Uyumlu ağ geçidinde `OCR_ACP_PARSER_BASE_URL` değerini API temel URL'sine ayarlayın.

Temel URL yol kuralları protokole bağlıdır:

| Protokol | `OCR_ACP_PARSER_BASE_URL` örneği | Gerçek istek URL'si |
| --- | --- | --- |
| `openai` | `https://gateway.example/v1` | `https://gateway.example/v1/chat/completions` |
| `anthropic` | `https://gateway.example` | `https://gateway.example/v1/messages` |

`/chat/completions` veya `/v1/messages` ile biten tam URL de verebilirsiniz; son ek yeniden eklenmez. Anthropic'te `/v1` ile biten temel URL, `/v1/v1/messages` üretir; ağ geçidinin gerektirdiği yolu kullanın.

| Ortam değişkeni | Başlatma seçeneği | Amaç |
| --- | --- | --- |
| `OCR_ACP_PARSER_PROVIDER` | `--parser-provider` | `openai` veya `anthropic` |
| `OCR_ACP_PARSER_MODEL` | `--parser-model` | Araç çağırma destekli model |
| `OCR_ACP_PARSER_BASE_URL` | `--parser-base-url` | İsteğe bağlı API temel URL'si |
| `OCR_ACP_PARSER_API_KEY` | Yok | Yalnızca ortamdan sağlanan ayrıştırıcı model anahtarı |

Desteklenen ayrıştırıcı protokolleri `openai` ve `anthropic` değerleridir; `openai-responses` ve `anthropic-bedrock` desteklenmez.

Anthropic ayrıştırıcı istekleri, zorunlu `submit_intent` araç çağrısının çalışması için Thinking'i açıkça kapatır. OCR inceleme modelinin Thinking ayarları değişmez.

Ayrıştırıcıyı yapılandırdıktan sonra yeni konuşma açıp deneyin:

```text
Review the changes in my current workspace.
Review the latest checked-out commit.
Scan internal/agent for potential issues.
```

İstek belirsizse çalıştırmadan önce açıklama sorusunu yanıtlayın. Modele sizin dilinizde sorması ve açıklaması söylenir; sabit doğrulama mesajları ve rapor etiketleri İngilizce kalır. Ayrıştırıcı kullanılamadığında eğik çizgi komutları çalışmaya devam eder. Eksik ayrıştırıcı yapılandırması açık bir başlangıç hatası üretir.

“Çalışma kopyasında seçili son commit” ifadesi `/review --commit HEAD` komutuna dönüşür. Oturum tek bir bekleyen açıklama isteği tutar. Uyumlu bilinen alanlar açıklamalar boyunca birikebilir; açık yeni değer eskisinin yerini alır. Başka inceleme türünün alanları karıştırılmaz. Tamamlama veya iptal durumu temizler; durum oturumlar arasında geri yüklenmez.

## Sonuçları okuma ve iptal {#read-results-and-cancel-work}

İlerleme, OCR CLI'nin mevcut günlüklerini ve son JSON'unu kullanır. Dosya grubu aşamaları, aşama süreleri veya model bekleme süresi tahmin edilmeden son bildirilen etkinlik ve geçen toplam süre gösterilir.

Tek bir **OCR review** veya **OCR scan** girdisi ilerlemeyi ve yerelde ölçülen süreyi gösterir. **Command:**, çalışma dizini ve uzunluğu sınırlı düz metin günlükleri görmek için genişletin. Tanılama ayrıntılarda kalır, ilerleme başlığının yerini almaz; komutlar ve günlükler konuşma gövdesinde tekrarlanmaz. İlk genişletme durumunu istemci belirler.

Son rapor önce tamamlanma durumunu, dosya ve bulgu sayılarını, önem derecesi dağılımını gösterir. Her bulguda önem derecesi ve dosya konumu içeren numaralı başlık, kategori, açıklama ve dil etiketli kod bloğu bulunur. Önerilen düzeltmelerin uygulanmadığı açıkça belirtilir. Bulgular yalnızca bir kez gösterilir. Geçerli konumlar dosyalara bağlanır; eksik dosyalar, geçersiz satırlar ve çalışma kökü dışındaki yollar düz metin kalır. Görünüm ve tıklama davranışı istemci sürümüne bağlıdır.

Kısmi başarısızlık ayrıntıları ve bütçe sınırı bildirimleri korunur. Alt bilgide varsa toplam token ve OCR'nin bildirdiği çalışma süresi görünür. Yürütme ayrıntıları ayrıca varsa araç çağrı sayısını ve girdi, çıktı, önbellek token'larını gösterir; olmayan veriler atlanır. Canlı sayaç yerel süreyi ölçer ve OCR süresinden farklı olabilir. Token toplamı yalnızca yeni yanıtı değil tüm model isteklerini kapsar.

İsteği durdurmak için istemcinin iptal düğmesini kullanın. Bağdaştırıcı görevi iptal eder ve yönettiği süreçlerin temizlenmesini bekler. Ayrıştırma dâhil tüm turu sınırlamak için Agent'ın `args` dizisine `"--turn-timeout", "10m"` ekleyin. Varsayılan `0` (tur sınırı yok) değeridir; ayrıştırıcı isteklerinin ayrı varsayılanı 15 saniyedir. Zaman aşımı sonrası yeniden denemeniz istenir.

## Sorun giderme {#troubleshooting}

| Belirti | Kontrol |
| --- | --- |
| Ajan başlamıyor | İki çalıştırılabilir dosyanın yol ve izinlerini kontrol edin. OCR için `ocr llm test` çalıştırın; eksik ayrıştırıcı ayarlarını kaldırın veya tüm zorunlu değerleri sağlayın. |
| Komutlar çalışıyor, doğal dil çalışmıyor | Ayrı `OCR_ACP_PARSER_*` değişkenlerini yapılandırın; sağlayıcı, model, anahtar ve ağ geçidi URL'sini kontrol edin. |
| Doğal dil ayrıştırma zaman aşımı | Ayrı varsayılan sınır 15 saniyedir; OCR henüz başlamamıştır. Tanılamadaki istek aşaması, HTTP durumu ve sağlayıcı yanıtından nedeni bulun. `--turn-timeout` artırmak ayrıştırıcı sınırını uzatmaz. Yeniden deneyin veya ayrıştırmayı atlamak için `/review`, `/scan` kullanın. |
| İnceleme başarısız veya kısmi | OCR inceleme/tarama ayrıntılarını genişletip son hata mesajını okuyun. Model yapılandırmasını ve sağlayıcı erişimini kontrol edin. |
| Görev çok uzun sürüyor | İptal edin, ilerleme ayrıntılarını inceleyin veya tur zaman aşımı ayarlayın. İncelemede `--effort low` tur sayısını azaltır. |
| Yeniden derleme sonrası eski davranış | İkili dosya yolunu kontrol edip yeni OpenCodeReview konuşması açın. Eski süreci kullanıyorsa ajanı veya istemciyi yeniden başlatın. |

Bağlantı hatalarında istemcinin ACP günlüklerini ve bağdaştırıcının stderr tanılamalarını inceleyin. Paylaşmadan önce anahtarları ve özel kaynak kodunu kaldırın.

## Yükseltme ve destek sınırları {#upgrade-and-support-boundaries}

Kaynağı güncelledikten sonra `make -C acp build` komutunu yeniden çalıştırıp yeni OpenCodeReview konuşması açın. Çalışan süreçler ve eski mesajlar güncellemeyi yüklemez. Oturum geri yükleme desteklenmez; yeni konuşma önceki açıklama isteğini korumaz.

Bağdaştırıcı şu anda yerel stdio aktarımı kullanır. HTTP, birden fazla çalışma alanı kökü, otomatik dosya düzenleme, görüntü ve ses istekleri desteklenmez. Diğer ACP istemcileri ayrı uyumluluk kontrolleri gerektirir; protokol testlerinin geçmesi her istemcinin arayüz davranışını doğrulamaz.

## Ayrıca bakın {#see-also}

- [Yapılandırma](../../configuration/) — OCR inceleme modeli ve sağlayıcı ayarları.
- [CLI başvurusu](../../cli-reference/) — inceleme ve tarama seçenekleri.
- [ACP'ye giriş](https://agentclientprotocol.com/get-started/introduction) — resmî Agent Client Protocol tanıtımı.

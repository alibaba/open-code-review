---
title: Telemetri
sidebar:
  order: 11
---

OCR yerleşik **OpenTelemetry** desteği sunar. Her inceleme, yapılandırılmış span'ler (izleme aralıkları), metrikler ve olaylar üretir. Bir toplayıcıya bağlandığında bu veriler “Ajan zamanı neye harcadı?”, “Hangi modelin maliyeti neydi?” ve “Bu çalıştırma neden başarısız oldu?” sorularını yanıtlar.

## Genel bakış {#overview}

Telemetri **varsayılan olarak kapalıdır**. Etkinleştirildiğinde OCR şunları dışa aktarır:

- **Span'ler** — üç süreç düzeyi span (`review.run`, `diff.parse`, `subtask.execute.group.<group-key>`), LLM istekleri (`llm.request`), araç yürütmeleri (`tool.execute.<tool-name>`) ve karar noktalarında kısa ömürlü `event.*` span'leri.
- **Metrikler** — inceleme süresi, incelenen dosyalar, üretilen yorumlar, LLM istekleri / token'ları / gecikmesi ve araç çağrıları / gecikmesi için toplu sayımlar ve histogramlar.
- **Olaylar** — `plan.skipped`, `token.threshold.exceeded`, `review.started` gibi span içindeki ayrı olaylar.

İki dışa aktarıcı desteklenir:

| Dışa aktarıcı | Kullanım |
|---|---|
| `console` | Kişisel kullanım / hata ayıklama. Span'leri okunabilir biçimde stdout'a yazar. |
| `otlp` | Sistem entegrasyonu. OTLP uyumlu toplayıcılara gönderir (Jaeger, Tempo, OTel Collector, Datadog Agent, …). |

## Telemetriyi etkinleştirme {#enabling-telemetry}

LLM uç noktası gibi telemetri de kalıcı yapılandırma veya ortam değişkenleriyle ayarlanır; çakışmada ortam değişkenleri önceliklidir.

### Yapılandırma dosyasıyla {#config-file-approach}

```bash
ocr config set telemetry.enabled        true
ocr config set telemetry.exporter       otlp
ocr config set telemetry.otlp_endpoint  localhost:4317
ocr config set telemetry.content_logging false
```

`~/.opencodereview/config.json` içindeki sonuç:

```json
{
  "telemetry": {
    "enabled": true,
    "exporter": "otlp",
    "otlp_endpoint": "localhost:4317",
    "content_logging": false
  }
}
```

### Ortam değişkenleriyle {#environment-variable-approach}

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317   # implies exporter=otlp
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc             # default; http/protobuf and http/json
                                                    # are also supported
export OTEL_SERVICE_NAME=open-code-review-prod      # optional; default: open-code-review
export OCR_CONTENT_LOGGING=0                        # reserved / currently a no-op (see Content logging)
```

`OTEL_EXPORTER_OTLP_ENDPOINT`, `exporter=otlp` seçimini de zorlar; tek seferlik `OTEL_EXPORTER_OTLP_ENDPOINT=… ocr review` çalıştırmaları için kullanışlıdır.

### Uç nokta biçimi {#endpoint-format}

Uç nokta yalın `host:port` veya şemalı olabilir. Şema aktarım güvenliğini belirler: `http://` düz metin, `https://` TLS kullanır. Yalın `host:port` TLS kullanır.

Varsayılan port protokole göre değişir: gRPC için `4317`, HTTP için `4318`:

```bash
# gRPC (default)
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317

# HTTP
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

HTTP protokollerinde uç nokta **temel URL**'dir; sinyale özgü yol sonuna eklenir. Örneğin `http://localhost:4318`, izleri `http://localhost:4318/v1/traces` adresine gönderir. Temel yol korunur; bir önek altında sunulan arka uçlar bunu gerektirir:

```bash
# traces go to http://collector:3000/api/public/otel/v1/traces
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_EXPORTER_OTLP_ENDPOINT=http://collector:3000/api/public/otel
```

gRPC'nin URL yolu yoktur; bu yalnızca HTTP protokolleri için geçerlidir.

## Dışa aktarılan veriler {#what-gets-exported}

### Span'ler {#spans}

Örnek inceleme span ağacı:

```
review.run
├── diff.parse
├── event.review.started                   (decision-point event)
├── event.grouping.skipped                 (when the change set is below the grouping thresholds)
├── subtask.execute.group.<group-key1>
│   ├── event.plan.skipped                 (when changes are below both thresholds)
│   ├── event.plan.failed                  (when plan phase errored)
│   ├── event.token.threshold.exceeded     (when prompt > 80% of max_tokens)
│   ├── main.loop                          (one span per review round)
│   │   ├── llm.request
│   │   └── tool.execute.<tool-name>
│   └── event.subtask.error                (when the subtask errored)
├── subtask.execute.group.<group-key2>
└── …
```

`subtask.execute.group.*` span'i dosya başına değil, incelenen **grup** başına üretilir; dosyalar incelemeden önce anlamsal olarak gruplanır. Grup anahtarı, sıralanıp virgülle birleştirilmiş dosya yollarıdır (tek dosyada tek yol).

Ana inceleme döngüsü LLM isteklerini `llm.request`, araç yürütmelerini `tool.execute.<tool-name>` span'leri olarak kaydeder. Toplam sayılar ve gecikmeler metrik olarak da kaydedilir. Karar noktası olayları mevcut bağlama bağlı kısa ömürlü `event.<name>` span'leridir.

Her span yararlı öznitelikler taşır:

| Span | Temel öznitelikler |
|---|---|
| `review.run` | `error` (çalıştırma başarısızsa ayarlanır) |
| `diff.parse` | `files.changed`, `lines.inserted`, `lines.deleted` |
| `subtask.execute.group.<group-key>` | `group.label`, `group.file_count`, `lines.changed`, `lines.changed.max_file` |
| `main.loop` | `group.label`, `round` |
| `llm.request` | `llm.model`, `llm.duration_ms`, `llm.total_tokens`, `llm.status` |
| `tool.execute.<tool-name>` | `tool.name`, `tool.duration_ms`, `tool.status` |
| `event.review.started` | `file.count`, `review.count`, `repo.dir` |
| `event.review.skipped` | `reason` (`too_large` / `deleted` / `no_supported_files`), `file.count`, `too_large.count` |
| `event.grouping.skipped` | `strategy`, `file.count`, `lines.changed`, `threshold.files`, `threshold.lines` |
| `event.plan.skipped` | `group.label`, `group.file_count`, `lines.changed`, `lines.changed.max_file`, `threshold`, `threshold.group` |
| `event.plan.failed` | `group.label`, `message` |
| `event.token.threshold.exceeded` | `group.label`, `tokens`, `max_tokens`, `round` |
| `event.subtask.error` | `group.label`, `error` |


### Metrikler {#metrics}

OCR, OTel ölçüm aracıyla toplayıcının birleştirdiği sayımları ve histogramları kaydeder:

| Metrik | Tür | Birim | Etiketler |
|---|---|---|---|
| `ocr.review.duration_seconds` | histogram | `s` | — |
| `ocr.files_reviewed_total` | sayaç | — | — |
| `ocr.comments_generated_total` | sayaç | — | — |
| `ocr.llm.requests_total` | sayaç | — | `model`, `status` (`ok` / `error`) |
| `ocr.llm.request_duration_seconds` | histogram | `s` | `model` |
| `ocr.llm.tokens_used` | sayaç | — | `model`, `type` (şu anda her zaman `total`) |
| `ocr.tool.calls_total` | sayaç | — | `tool.name`, `status` (`ok` / `error`) |
| `ocr.tool.execution_duration_seconds` | histogram | `s` | `tool.name` |


### Olaylar {#events}

Olaylar karar noktalarında kısa ömürlü `event.<name>` span'leri olarak oluşur. Tam liste:

| Olay | Anlam |
|---|---|
| `review.started` | Farklar yüklendi; incelenecek dosya sayısı belli. |
| `no.files.changed` | Fark sıfır dosya içeriyor. |
| `review.skipped` | Seçimden sonra incelenecek dosya kalmadı; `reason`: `too_large`, `deleted` veya `no_supported_files`. |
| `grouping.skipped` | Değişiklik kümesi `GROUPING_MIN_FILES` değerinden az dosya içerdiği için gruplama çağrısı atlandı. `strategy`, değişen satır sayısı `GROUPING_BUNDLE_LINE_THRESHOLD` altındaysa `bundle_all` (tüm dosyalar tek grup), eşit veya üstündeyse `per_file` (dosya başına grup) olur. Tek dosyalı küme, bölünecek bir şey olmadığından eşiklerden bağımsız olarak her zaman `per_file` olur; terminal satırı olmadan yalnızca burada raporlanır. |
| `plan.skipped` | Grup iki plan eşiğinin de altında: en çok değişen dosyada `PLAN_MODE_LINE_THRESHOLD` değerinden az satır değişmiş ve 2+ dosyalı gruplarda toplam `PLAN_MODE_GROUP_LINE_THRESHOLD` altında. |
| `plan.failed` | Plan aşaması hata verdi; ana döngü plansız çalıştı. |
| `token.threshold.exceeded` | İstem token sayısı, girdi tavanı `MAX_TOKENS` değerinin %80'ini aştı; grup atlandı. |
| `subtask.error` | Grup alt görevi hata verdi; `Error` span durumuyla yayımlanır. |

Bunları, kullanıcı fark etmeden inceleme kalitesindeki düşüşler için uyarı oluşturmakta kullanın.

## İçerik kaydı {#content-logging}

Telemetri, LLM trafiğinin **yapısını** (sayılar, süreler, durumlar) aktarır; gerçek istem veya yanıtları **asla** aktarmaz. OCR, LLM mesaj içeriğini span veya olaylara eklemez. Süreçten çıkan veri yalnızca yukarıda belgelenen metrik / olay şemasıdır.

`content_logging` anahtarı ve `OCR_CONTENT_LOGGING=1` ortam değişkeni yapılandırma katmanından geçirilir; ancak şu anda istem içeriği gönderen herhangi bir kod yolunu **kontrol etmez**. Ayrılmış bir seçenek olarak değerlendirin.

LLM'ye gönderilenleri ve yanıtları incelemek için [Oturum görüntüleyicisi](../viewer/)nin okuduğu yerel JSONL kayıtlarını kullanın. Bunlar yalnızca `~/.opencodereview/` altında diskte bulunur, toplayıcıya gönderilmez.

Daha ayrıntılı hata ayıklama için `OCR_RAW_LOGGING=1` ayarlayın. Her LLM çağrısının ham istek ve yanıt gövdeleri `~/.opencodereview/raw/` altında saklanır; oturum kayıtlarından daha ayrıntılıdır. Varsayılan olarak kapalıdır. Akış etkinse yanıt gövdesinin tamamı diğer işlem adımlarına verilmeden önce okunur. Başlıklar gizlenir; istek ve yanıt gövdeleri olduğu gibi kaydedilir.

## Örnek kurulumlar {#recipes}

### Yerel hata ayıklama için console dışa aktarıcısı {#console-exporter-for-local-debugging}

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter console
ocr review --commit HEAD
```

Span'ler stdout'a okunabilir biçimde yazılır. Uzun çalıştırmaları okumak için `less` komutuna aktarın.

### Tempo + Prometheus ile OTel Collector {#otel-collector-with-tempo-prometheus}

```yaml
# otel-collector-config.yaml
receivers:
  otlp:
    protocols: { grpc: { endpoint: 0.0.0.0:4317 } }

exporters:
  otlp/tempo:
    endpoint: tempo:4317
    tls: { insecure: true }
  prometheus:
    endpoint: 0.0.0.0:9464

service:
  pipelines:
    traces:  { receivers: [otlp], exporters: [otlp/tempo] }
    metrics: { receivers: [otlp], exporters: [prometheus] }
```

Ardından kabukta:

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
ocr review --from main --to feature/branch
```

Tempo'yu açın → `service.name=open-code-review` ile arayın → tüm span ağacı için bir ize tıklayın.

### Datadog {#datadog}

Datadog Agent'ın OTLP alıcısı varsayılan olarak OTLP/gRPC kullanır:

```bash
export OCR_ENABLE_TELEMETRY=1
export OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317
export OTEL_SERVICE_NAME=open-code-review
```

Span'ler APM altında hizmet adıyla; LLM metrikleri Metrics altında yukarıdaki etiketlerle görünür.

### CI çalıştırması, sonuçlar panonuzda {#ci-run-results-in-your-dashboard}

Süreç adımına ortam değişkenlerini ekleyin:

```yaml
- name: Code review
  env:
    OCR_LLM_URL: ${{ secrets.OCR_LLM_URL }}
    OCR_LLM_TOKEN: ${{ secrets.OCR_LLM_TOKEN }}
    OCR_LLM_MODEL: claude-opus-4-6
    OCR_ENABLE_TELEMETRY: "1"
    OTEL_EXPORTER_OTLP_ENDPOINT: ${{ vars.OTEL_COLLECTOR_URL }}
    OTEL_SERVICE_NAME: open-code-review-ci
  run: ocr review --from origin/main --to HEAD --audience agent
```

`OTEL_SERVICE_NAME`, CI izlerini geliştiricilerin çalıştırmalarından ayırır.

## Yapılandırma önceliği {#resolution-priority}

OCR son telemetri yapılandırmasını şu sırayla oluşturur:

1. Varsayılanlar (`enabled=false`, `exporter=console`, uç nokta yok).
2. `~/.opencodereview/config.json` içindeki `telemetry.*` anahtarları.
3. Ortam değişkenleri (en yüksek öncelik; dosyayı **geçersiz kılar**).

Dosyada `telemetry.enabled=false` bırakıp çalıştırma başına `OCR_ENABLE_TELEMETRY=1` ile açabilirsiniz.

## Örnekleme ve ek yük {#sampling-and-overhead}

OCR **her şeyi** dışa aktarır. Örnekleme ayarı yoktur; OTel örneklemesinden toplayıcınız sorumludur. Tipik bir incelemede:

- 1 `review.run` + 1 `diff.parse` span'i; incelenen grup başına 1 `subtask.execute.group.<group-key>` (ve `plan.execute` / `main.loop` / `review_filter.execute` alt span'leri); karar olayı başına 1 kısa ömürlü `event.*`; ayrıca `llm.request` ve `tool.execute.<tool-name>` span'leri.
- Toplam span sayısı yalnızca dosya sayısına değil; incelenen gruplara, inceleme turlarına, LLM isteklerine ve araç yürütmelerine bağlıdır.

Aktarım **toplu ve asenkrondur**; inceleme döngüsünü engellemez. Toplayıcıya ulaşılamazsa OCR uyarı kaydedip devam eder; inceleme normal çıktısını üretir.

## Sorun giderme {#troubleshooting}

| Belirti | Olası neden |
|---|---|
| Hiçbir şey aktarılmıyor | `OCR_ENABLE_TELEMETRY` / `telemetry.enabled` ayarlı değil. Varsayılan **kapalıdır**. |
| OTLP yerelde çalışıyor, üretimde çalışmıyor | Protokolü kontrol edin. Varsayılan gRPC'dir; birçok yönetilen arka uç yalnızca HTTP üzerinden OTLP kabul eder. `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf` ve HTTP portunu (`4317` yerine `4318`) kullanın. |
| Veri gelmiyor, hata yok | Dışa aktarıcı ihtiyaç anında oluşturulur; yanlış uç nokta başlangıçta değil aktarım sırasında sessizce başarısız olur. Toplayıcının erişim günlüğündeki yolu kontrol edin. HTTP'de uç nokta temel URL olduğundan `http://host:4318`, `http://host:4318/v1/traces` yoluna gönderir. |
| Span var, metrik yok | Bazı toplayıcılarda yalnızca iz süreci varsayılan olarak açıktır; yapılandırmaya `metrics` süreci ekleyin. |
| Span'lerde istemler yok | OCR istem içeriğini telemetriye eklemez; [İçerik kaydı](#content-logging) bölümüne bakın. Kayıtları [Oturum görüntüleyicisi](../viewer/) ile inceleyin. |

## Ayrıca bakın {#see-also}

- [Yapılandırma](../configuration/) — `telemetry.*` ad alanının tüm anahtarları.
- [Mimari](../architecture/) — her span'in neyi ölçtüğü.
- [OpenTelemetry belgeleri](https://opentelemetry.io/docs/) — toplayıcı kurulumu ve dışa aktarıcılar.

---
title: Hızlı başlangıç
sidebar:
  order: 3
---

İlk kod incelemenizi birkaç dakika içinde çalıştırın.

## Ön koşullar {#prerequisites}

- **Git ≥ 2.41**
- **Node.js ≥ 18**
- **LLM API anahtarı** ([Yetkilendirme modu](../integrations/delegate/) kullanılıyorsa gerekmez)

## Adım 1 — CLI'yi yükleyin {#step-1-install-the-cli}

```bash
npm install -g @alibaba-group/open-code-review
```

```bash
ocr version
```

> Diğer yöntemler için [Yükleme](../installation/) sayfasına bakın.

## Adım 2 — Bir LLM yapılandırın {#step-2-configure-an-llm}

> [Yetkilendirme modu](../integrations/delegate/) kullanıyorsanız (örneğin Claude Code içinde), modeli ana ajan sağlar; 4. adıma geçin.

```bash
ocr config provider
```

Bu komut yerleşik veya özel bir sağlayıcı seçmenizi, API anahtarını girmenizi ve modeli belirlemenizi sağlar. Ayarları yapılandırma dosyasına kaydeder, ardından uç noktayı doğrulamak için bir kez `ocr llm test` çalıştırır. Modeli daha sonra değiştirmek için:

```bash
ocr config model
```

### Alternatif: etkileşimsiz komut {#alternative-non-interactive-command}

CI veya TUI bulunmayan ortamlarda aynı yapılandırmayı doğrudan `ocr config set` ile yazın:

```bash
ocr config set provider                    anthropic
ocr config set model                       claude-opus-4-6
ocr config set providers.anthropic.api_key sk-ant-xxxxxxxxxx
```

## Adım 3 — Bağlantıyı sınayın {#step-3-test-connectivity}

```bash
ocr llm test
```

`no valid LLM endpoint configured` gibi bir hata alırsanız 2. adımdaki yapılandırmayı kontrol edin. 401 / 403, token'ın yanlış veya süresinin dolmuş olduğunu gösterir.

## Adım 4 — İlk incelemenizi çalıştırın {#step-4-run-your-first-review}

Herhangi bir Git deposuna geçip çalıştırın:

```bash
cd path/to/your-repo

# Çalışma alanı — hazırlanmış, hazırlanmamış ve izlenmeyen değişiklikler (varsayılan)
ocr review

# Dal aralığı — feature-branch dalının main'den ayrıldıktan sonraki değişiklikleri (merge-base)
ocr review --from main --to feature-branch

# Tek commit — commit'in getirdiği fark
ocr review --commit abc123
```

> Tüm `ocr review` seçenekleri (eşzamanlılık, çıktı biçimi, çıktı hedefi, gereksinim bağlamı ve diğerleri) ile diğer alt komutlar için [CLI başvurusu](../cli-reference/) sayfasına bakın.

### Önce nelerin inceleneceğini görmek ister misiniz? {#want-to-see-what-would-be-reviewed-first}

```bash
ocr review --preview              # çalışma alanı
ocr review -c abc123 --preview    # commit
```

### Sistemler için JSON çıktısı {#json-output-for-systems}

`--audience agent`, insanlara yönelik ilerleme arayüzünü gizler. Böylece stdout yalnızca JSON / son özeti içerir; çağıran ajan veya CI betiği için uygundur.

```bash
ocr review --format json --audience agent > review.json
```

## Ayrıca bakın {#see-also}

- [Yükleme](../installation/) — tüm yükleme yöntemleri ve OCR durum dizini.
- [Yapılandırma](../configuration/) — tüm ortam değişkenleri, yapılandırma anahtarları ve yerleşik sağlayıcılar.
- [CLI başvurusu](../cli-reference/) — tüm alt komutlar, seçenekler ve çıktı modları.
- [İnceleme kuralları](../review-rules/) — nelerin inceleneceğini özelleştirin.
- [Entegrasyonlar](../integrations/agent-skill/) — OCR'yi Claude Code, ajan becerisi veya CI içine ekleyin.
- [Sık sorulan sorular](../faq/) — bilinen hatalar ve çözümleri.

---
title: Gereksinimlere göre inceleme
sidebar:
  order: 8
---

Bir değişikliğin kabul ölçütlerini OCR'ye vererek kod hatalarının yanı sıra uygulamanın iş gereksinimlerini karşılayıp karşılamadığını da inceletebilirsiniz. Önerilen yöntem, gereksinimleri yerel bir Markdown dosyasına yazıp `--background-file` ile iletmektir.

## Bir değişiklik için gereksinim dosyası {#requirements-file-for-a-change}

Tek bir değişiklik için `docs/requirements/checkout.md` gibi kısa bir yerel Markdown dosyası hazırlayın:

```markdown
# Sipariş tamamlama gereksinimleri

Kapsam: src/checkout/**
Değişiklikleri aşağıdaki ölçütlere göre inceleyin ve bulgularda ilgili gereksinim kimliğini belirtin.

- PAY-1: Sepet boşsa siparişin tamamlanmasına izin verme.
- PAY-2: Toplam tutarı sunucuda hesapla; istemcinin gönderdiği toplam tutara güvenme.
- PAY-3: Aynı idempotency anahtarıyla yinelenen bir istek ikinci bir tahsilata yol açmamalı.

Kapsam dışı: İadeler ve abonelik ödemeleri.
```

OCR yapılandırıldıktan sonra, `main` dalının bulunduğu deponun kök dizininde şu komutu çalıştırın:

```bash
ocr review --from main --to HEAD --background-file docs/requirements/checkout.md
```

`--background-file` tüm inceleme için bağlam sağlar. OCR'yi bir alt dizinde çalıştırsanız bile göreli yollar **deponun kök dizinine** göre çözümlenir. Dosyanın tam konumunu da belirtebilirsiniz. `--background-file` ve `--background` seçeneklerini birlikte kullanırsanız OCR yalnızca dosyadaki metni okur. `--background` ile yazdığınız metni dikkate almaz. Yalnızca gerekli bilgileri ekleyin: OCR, temizlenmiş metin 2.000 karakteri aşarsa uyarı verir; 8.000 karakteri aşarsa girdiyi reddeder. Dosyanın temizleme öncesindeki boyutu en fazla 1 MiB olmalı ve temizleme sonrasında metin boş kalmamalıdır.

## Kurum içi bilgi tabanındaki gereksinimler {#requirements-from-an-internal-knowledge-base}

İlgili gereksinimleri, kısıtları ve kabul ölçütlerini bilgi tabanından kısa bir yerel Markdown dosyasına aktarın ve `--background-file` ile verin. Bağlam metninde bir dosyanın adını veya bir bağlantıyı belirtmek, OCR'nin o belgeyi otomatik olarak yüklemesini ya da bağlantıları kendiliğinden izlemesini sağlamaz. Ölçütlerin kendisini ekleyin ve bulguları gereksinim belgesiyle ve testlerle karşılaştırın.

## Sınırlamalar {#limitations}

OCR, gereksinimleri tek bir inceleme için bağlam olarak kullanır. Şu anda başvuru belgelerini inceleme kurallarından bağımsız olarak bir modül veya dizinle ilişkilendirmenin özel bir yolu yoktur. Yol kuralları, eşleşen dosyalara hangi kural metninin uygulanacağını seçer; başvuru belgesi eklemek için kullanılmaz. Bilgi tabanıyla ilgili kullanım senaryosu olgunlaşırsa buna yönelik destek ayrıca tasarlanabilir.

## Ayrıca bakınız {#see-also}

- [CLI başvurusu](../cli-reference/) — `ocr review --background-file` ve diğer inceleme seçenekleri.
- [İnceleme kuralları](../review-rules/) — kural katmanları, yol kuralları ve kuralların nasıl seçildiği.

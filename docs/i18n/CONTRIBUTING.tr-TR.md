# OpenCodeReview'a katkıda bulunma

OpenCodeReview'a katkıda bulunmak istediğiniz için teşekkür ederiz. İster bir yazım hatasını düzeltin, ister bir hata bildirin, ister yeni bir özellik geliştirin; her katkı değerlidir.

[English](../../CONTRIBUTING.md) | [简体中文版](CONTRIBUTING.zh-CN.md) | [日本語版](CONTRIBUTING.ja-JP.md) | [한국어](CONTRIBUTING.ko-KR.md) | [Русский](CONTRIBUTING.ru-RU.md) | Türkçe

## Davranış kuralları

Bu projeye katılarak herkesin birbirine saygı gösterdiği ve kendini dışlanmış hissetmediği bir ortamı korumayı kabul edersiniz. Tüm iletişiminizde nazik ve yapıcı olun.

## Nasıl katkıda bulunabilirsiniz?

Katkıda bulunmanın kod yazmak dışında da pek çok yolu var:

- **Hata bildirin.** Çalışmayan bir şey mi buldunuz? Hatayı yeniden oluşturma adımlarını içeren bir kayıt (issue) açın.
- **Özellik önerin.** Bir iyileştirme fikriniz varsa [GitHub Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas) üzerinden tartışma başlatabilir veya [özellik isteği](https://github.com/alibaba/open-code-review/issues/new?template=feature_request.yml) açabilirsiniz.
- **Belgeleri iyileştirin.** Yazım hatalarını düzeltin, belirsiz anlatımları netleştirin veya örnek ekleyin. Sorunları [belge sorunu bildirerek](https://github.com/alibaba/open-code-review/issues/new?template=docs_report.yml) de iletebilirsiniz.
- **Pull request'leri (PR) inceleyin.** Diğer katkıda bulunanların kodunu incelememize yardımcı olun.
- **Kod yazın.** Hataları düzeltin, özellik ekleyin veya performansı artırın.

## Başlarken

### Ön koşullar

- [Go 1.26.10 veya üstü](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### Kurulum

```bash
# 1. GitHub'da depoyu kendi hesabınıza çatallayın (fork).

# 2. <your-username> yerine kullanıcı adınızı yazarak çatalınızı klonlayın.
git clone https://github.com/<your-username>/open-code-review.git
cd open-code-review

# 3. Ana depodaki güncellemeleri alabilmek için upstream uzak deposunu ekleyin.
git remote add upstream https://github.com/alibaba/open-code-review.git

# 4. Projeyi derleyin.
make build

# 5. Testleri çalıştırın.
make test
```

Tüm adımlar başarılıysa katkıda bulunmaya hazırsınız.

> **Not:** `upstream` katkıda bulunanlar için salt okunurdur. Ana depodaki güncel değişiklikleri buradan alabilirsiniz, ancak bu depoya doğrudan gönderim yapamazsınız. Commit'lerinizi kendi çatalınıza (`origin`) gönderin ve bir PR açın.

### Satır sonları

Proje, `.gitattributes` aracılığıyla LF satır sonlarını zorunlu kılar. Git'i satır sonlarını otomatik olarak dönüştürecek şekilde ayarlayın:

```bash
git config core.autocrlf input
```

Böylece CRLF satır sonları commit sırasında LF'ye dönüştürülür ve CI'da satır sonlarından kaynaklanan sorunlar önlenir.

## Geliştirme iş akışı

### Dal oluşturma

Çalışmanız için `main` dalından yeni bir dal oluşturun:

```bash
git checkout main
git pull upstream main
git checkout -b feat/your-feature-name
```

Değişikliğin türünü dal adının önekiyle belirtin:

| Önek | Kullanım amacı |
| --- | --- |
| `feat/` | Yeni özellik |
| `fix/` | Hata düzeltmesi |
| `docs/` | Yalnızca belge değişiklikleri |
| `refactor/` | Davranışı değiştirmeden kodu yeniden düzenleme |
| `test/` | Test ekleme veya güncelleme |
| `chore/` | Derleme, CI veya geliştirme araçlarıyla ilgili değişiklikler |

### Commit mesajları

[Conventional Commits](https://www.conventionalcommits.org/) biçimini kullanın:

```
<tür>(<kapsam>): <kısa özet>

[isteğe bağlı açıklama]
```

Örnekler:

```
feat(viewer): add session search
fix(llm): handle timeout errors in Anthropic API calls
docs(README): update configuration examples
```

`PR Commit Policy`, PR başlığını ve her commit mesajının tamamını (açıklama dâhil) büyük/küçük harf ayrımı yapmadan denetler. `[skip ci]`, `[ci skip]`, `[no ci]`, `[skip actions]` veya `[actions skip]` ifadelerini kullanmayın; `skip-checks: true` ya da `skip-checks:true` içeren bir satır da eklemeyin.

Bu denetim başarısız olursa PR başlığını düzenleyin veya çalıştığınız dalda ilgili commit mesajlarını yeniden yazın. Kurala uygun yeni bir commit eklemek önceki ihlalleri gidermez. PR açıklaması bu denetime tabi değildir; bu talimatlardan orada söz edebilirsiniz.

### Lisans başlıkları

Her kaynak dosyasında (`.go`, `.js`, `.mjs`, `.ts`, `.tsx`, `.kt`, `.kts`, `.sh`, `.py`, `.css`) bir SPDX lisans başlığı bulunmalıdır. Yeni dosyalar oluşturduktan sonra şu komutu çalıştırın:

```bash
make license-add
```

Bu komut gerekli başlığı otomatik olarak ekler. Başlığı eksik dosyalar içeren PR'lar CI denetiminden geçmez.

### Kod kalitesi

Değişikliklerinizi göndermeden önce tüm denetimlerin başarılı olduğunu doğrulayın:

```bash
# Kodu biçimlendirin, statik denetimleri çalıştırın ve lisans başlıklarını doğrulayın.
make check

# Testleri veri yarışlarını algılama seçeneğiyle çalıştırın.
make test

# Projenin başarıyla derlendiğini doğrulayın.
make build
```

### Proje yapısı

```
├── acp/                  # CLI için ACP bağdaştırıcısı
├── bin/                  # NPM sarmalayıcısı
├── cmd/opencodereview/   # CLI giriş noktası
├── docs/                 # README ve CONTRIBUTING çevirileri
├── examples/             # CI/CD entegrasyon örnekleri
├── extensions/           # Düzenleyici eklentileri (VS Code ve IDEA)
├── imgs/                 # Logo, kıyaslama ve öne çıkan özelliklerin görselleri
├── internal/
│   ├── agent/            # İnceleme ajanının çalışma mantığı
│   ├── chatgptauth/      # ChatGPT OAuth ve kimlik bilgileri yönetimi
│   ├── config/           # Yapılandırma yönetimi
│   ├── delegate/         # Görevlendirme modunun inceleme tanımları (LLM kullanmaz)
│   ├── diff/             # Git farklarının ayrıştırılması
│   ├── gitcmd/           # Git alt süreçlerini belirlenmiş sınırlar içinde çalıştırır
│   ├── llm/              # LLM API istemcisi (Anthropic, Bedrock ve OpenAI)
│   ├── llmloop/          # Her alt görev için ortak araç kullanım döngüsü
│   ├── mcp/              # MCP sunucusu istemcisi ve araç bağdaştırıcısı
│   ├── model/            # Veri modelleri
│   ├── pathutil/         # Yolları standartlaştırma ve dizin sınırlarını denetleme
│   ├── release/          # Sürüm dosyalarının URL adlandırma denetimleri
│   ├── scan/             # Dosyanın tamamını kapsayan kod incelemesi (ocr scan)
│   ├── session/          # İnceleme oturumlarının yönetimi
│   ├── stdout/           # Değiştirilebilir standart çıktı yazıcısı
│   ├── suggestdiff/      # Önerileri göstermek için satır düzeyinde farklar
│   ├── telemetry/        # OpenTelemetry entegrasyonu
│   ├── tool/             # Yerleşik araçlar (file_read, code_search vb.)
│   └── viewer/           # Web arayüzünde oturum görüntüleyici
├── npm/                  # Platforma özgü, isteğe bağlı ikili dosya paketleri
├── pages/                # Projenin tanıtım sitesi
├── plugins/              # Kodlama ajanları için eklentiler
├── scripts/              # Kurulum, yayımlama ve denetim betikleri
└── skills/               # Ajan becerilerinin bildirim dosyaları
```

## Yapay zekâ destekli geliştirme

Geliştirme sırasında yapay zekâ kullanabilirsiniz; katkıda bulunmayı kolaylaştırmasına seviniriz. Ancak bir modelin ürettiği kodu okumadan, gereksiz kısımlarını ve hatalarını gidermeden doğrudan commit'e eklemenizi kabul edemeyiz. Bu tür gönderimler incelemeyi yavaşlatır ve PR'ın ilerlemesini zorlaştırır.

Çalışmanızda yapay zekâdan yararlandıysanız aşağıdaki kurallara uyun.

### Kurallar

1. **İlk açtığınız issue veya PR'da yapay zekâ ya da büyük dil modeli (AI/LLM) kullandığınızı açıkça belirtin; hangi araçları ve modelleri kullandığınızı da yazın.**
2. Yapay zekânın yazdığı her kod satırını anlamalı ve ne yaptığını bilmelisiniz.
3. İnceleyen kişi bir değişikliğin nedenini sorduğunda, kodu siz de yazmış olsanız yapay zekâ da yazmış olsa, nedenini kendiniz açıklayabilmelisiniz. Projeyi sürdürenlerin sorularına ve inceleme yorumlarına verdiğiniz yanıtlar kendi anlayışınıza dayanmalıdır. Yapay zekâyı yalnızca çeviri veya anlatımı iyileştirme amacıyla kullanabilirsiniz; sizin yerinize yanıt üretmesi için değil.
4. PR'ınızda `AI generated -> fixed -> fixed -> fixed` gibi tekrarlanan döngüler olmamalıdır. Bu, üretilen kodu kendiniz incelemek yerine her sorun çıktığında düzeltmeyi yeniden yapay zekâya bıraktığınızı gösterebilir.
5. Herhangi bir üyeden inceleme istemeden önce yapay zekânın ürettiği tüm kodu, metni ve diğer içerikleri kendiniz gözden geçirmelisiniz.
6. Commit'leri yapay zekâya veya bir dil modeline atfetmeyin. Buna commit mesajının sonuna `Assisted-by`, `Co-developed-by` veya benzeri bir ibare eklemek de dâhildir.
7. Gereğinden uzun commit mesajları yazmayın. Önemli bilgileri, arayüzde daraltılmış olarak görünen commit mesajları yerine PR açıklamasına yazın.
8. Bu kuralların tamamını uygulamak istemiyorsanız veya uygulayamıyorsanız lütfen issue ya da PR'ınızı kapatın.

Teşekkürler!

## Belgelere katkıda bulunma

Belgeler OpenCodeReview'ın önemli bir parçasıdır. README dosyalarını, kod içi yorumları, yapılandırma örneklerini ve kullanıcıların gördüğü tüm metinleri iyileştiren katkıları memnuniyetle karşılarız.

### Neler belge katkısı sayılır?

- Yazım ve dil bilgisi hatalarını ya da bozuk bağlantıları düzeltmek
- Anlaşılması güç açıklamaları netleştirmek veya eksik bağlamı eklemek
- Komutlar ya da yapılandırma seçenekleri için kullanım örnekleri eklemek
- Örneğin bir özellik değiştiğinde artık güncel olmayan içeriği yenilemek
- `docs/i18n/` dizinindeki README dosyalarını ve katkı kılavuzlarını çevirmek veya mevcut çevirileri iyileştirmek

### Belge değişikliklerinde iş akışı

1. Bir sorun fark ettiyseniz ancak kendiniz düzeltmeyi planlamıyorsanız [belge sorunu bildirin](https://github.com/alibaba/open-code-review/issues/new?template=docs_report.yml).
2. Düzeltmeyi kendiniz yapmak istiyorsanız depoyu çatallayın, değişiklikleri yapın ve `docs/` önekiyle adlandırdığınız daldan PR açın (örneğin `docs/fix-config-example`).
3. Yalnızca belgeleri değiştiren PR'larda test değişikliği gerekmez; ancak eklediğiniz komutların ve kod örneklerinin doğruluğunu kontrol edin.

### Belge dosyaları

| Dosya | Amaç |
| --- | --- |
| `README.md` | Ana proje belgesi (İngilizce) |
| `docs/i18n/README.zh-CN.md` | Çince çeviri |
| `docs/i18n/README.ja-JP.md` | Japonca çeviri |
| `docs/i18n/README.ko-KR.md` | Korece çeviri |
| `docs/i18n/README.ru-RU.md` | Rusça çeviri |
| `docs/i18n/README.tr-TR.md` | Türkçe çeviri |
| `docs/i18n/README.fr-FR.md` | Fransızca çeviri |
| `CONTRIBUTING.md` | Katkı kılavuzu (İngilizce) |
| `docs/i18n/CONTRIBUTING.zh-CN.md` | Katkı kılavuzu (Çince) |
| `docs/i18n/CONTRIBUTING.ja-JP.md` | Katkı kılavuzu (Japonca) |
| `docs/i18n/CONTRIBUTING.ko-KR.md` | Katkı kılavuzu (Korece) |
| `docs/i18n/CONTRIBUTING.ru-RU.md` | Katkı kılavuzu (Rusça) |
| `docs/i18n/CONTRIBUTING.tr-TR.md` | Katkı kılavuzu (Türkçe) |

## Değişiklikleri gönderme

### Issue açma

Kapsamlı bir değişiklik üzerinde çalışmaya başlamadan önce yaklaşımınızı tartışmak için bir issue açın. Böylece aynı işin iki kez yapılmasını önler ve katkınızın projenin hedefleriyle uyumlu olmasını sağlarsınız.

Hata bildirirken şunları ekleyin:

1. OpenCodeReview sürümü (`ocr version`)
2. İşletim sistemi ve işlemci mimarisi
3. Hatayı yeniden oluşturma adımları
4. Beklenen ve gerçekleşen davranış
5. İlgili günlük kayıtları veya hata mesajları

### Pull request süreci

1. **PR'ın kapsamını dar tutun.** Her PR tek bir mantıksal değişiklik içersin. Birbirinden bağımsız değişiklikleri ayrı PR'larla gönderin.
2. **Test yazın.** Davranış değişiklikleri için test ekleyin veya mevcut testleri güncelleyin.
3. **Belgeleri güncelleyin.** Değişikliğiniz kullanıcıların gördüğü davranışı etkiliyorsa ilgili belgeleri güncelleyin.
4. **CLA'yı imzalayın.** PR'ın birleştirilebilmesi için katkıda bulunan herkesin Katkıcı Lisans Sözleşmesi'ni imzalaması gerekir (aşağıya bakın).
5. **PR şablonunu doldurun.** Değişikliğinizin ne yaptığını ve neden gerekli olduğunu açıklayın.

### PR başlığının biçimi

Commit mesajlarında olduğu gibi Conventional Commits biçimini kullanın:

```
feat(viewer): add session search
```

### İnceleme süreci

- Projeyi sürdürenlerden biri genellikle birkaç iş günü içinde PR'ınızı inceler.
- Değişiklik isteyebiliriz. Bu, birlikte çalışmanın doğal bir parçasıdır; olumsuz bir tutumun işareti değildir.
- Onaydan sonra projeyi sürdürenlerden biri PR'ınızı birleştirir.

## PR incelemesini hızlandırmak için öneriler

PR'ınızın kısa sürede incelenip birleştirilmesi için şunlara dikkat edin:

- **CLA'yı erkenden imzalayın.** İlk kez katkıda bulunanlar bazen CLA botunun yorumunu gözden kaçırdığı için süreçleri durur. Bot size bildirdiğinde Katkıcı Lisans Sözleşmesi'ni imzalayın; imza olmadan PR'ınız birleştirilemez.
- **Tüm CI denetimlerinin geçtiğinden emin olun.** Denetimleri başarısız olan PR'lar incelenmez. Sorunları erken yakalamak için değişikliklerinizi göndermeden önce yerel ortamda `make test` ve `make build` çalıştırın.
- **Değişiklikleri küçük ve tek bir amaca yönelik tutun.** Tek bir işi iyi yapan PR'ı incelemek, ilgisiz değişiklikleri bir araya getiren bir PR'ı incelemekten çok daha kolaydır. Küçük PR'lar daha hızlı incelenir ve birden fazla düzeltme turu gerektirme olasılıkları daha düşüktür.
- **Açık ve doğru bir açıklama yazın.** *Neyin* değiştiğini ve *neden* değiştiğini anlatın. Açıklama, kodda gerçekten yapılan değişikliklerle örtüşmelidir; aksi hâlde inceleyenlerin güveni zedelenir. Kapsam çalışma sırasında değiştiyse inceleme istemeden önce açıklamayı güncelleyin.
- **Davranış değişiklikleri için test ekleyin.** Testi olmayan yeni özellikler ve hata düzeltmeleri soru işareti yaratır. Testler doğruluğu gösterir ve inceleyenlerin amaçlanan davranışı anlamasına yardımcı olur.
- **Mevcut kodun yaklaşımını izleyin.** Çevredeki kodun biçimine, adlandırma kurallarına ve mimarisine uyun. Tutarlılık incelemeyi kolaylaştırır ve yalnızca biçimle ilgili yorumlara gerek bırakmaz.
- **Geri bildirime zamanında yanıt verin.** İnceleyen kişi değişiklik istediğinde talebi gecikmeden ele alın. Katılmıyorsanız yorumu yanıtsız bırakmak yerine gerekçenizi açıklayın.

## Katkıcı Lisans Sözleşmesi (CLA)

Katkılarınızı birleştirebilmemiz için Alibaba Açık Kaynak Katkıcı Lisans Sözleşmesi'ni imzalamanız gerekir. Böylece proje kendi lisans koşulları altında dağıtılabilir.

İlk PR'ınızı açtığınızda CLA botu izlenecek adımları bir yorumda paylaşır. Bağlantıyı izleyerek elektronik imzanızı verebilirsiniz; bu işlem yalnızca bir dakika sürer.

## İlk kez katkıda bulunanlar için

Projede yeni misiniz? Şu etiketleri taşıyan kayıtlara bakabilirsiniz:

- [`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue) — Başlamak için uygun, küçük ve kapsamı belirli işler.
- [`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted) — Topluluğun yardımını beklediğimiz konular.

Başlamak için birkaç öneri:

- Hata mesajlarını ve CLI çıktısını iyileştirmek
- Henüz sınanmamış kod yolları için test yazmak
- Belgeleri iyileştirmek

## Topluluk

- **Hata bildirimleri:** [GitHub Issues](https://github.com/alibaba/open-code-review/issues)
- **Özellik önerileri:** [GitHub Discussions (Ideas)](https://github.com/alibaba/open-code-review/discussions/categories/ideas) üzerinden tartışma başlatın veya [özellik isteği](https://github.com/alibaba/open-code-review/issues/new?template=feature_request.yml) açın.
- **Sorular ve yardım:** OpenCodeReview kullanımıyla ilgili sorularınızı [GitHub Discussions](https://github.com/alibaba/open-code-review/discussions) üzerinden sorabilirsiniz.

## Lisans

OpenCodeReview'a katkıda bulunarak katkılarınızın [Apache Lisansı 2.0](../../LICENSE) kapsamında lisanslanmasını kabul edersiniz.

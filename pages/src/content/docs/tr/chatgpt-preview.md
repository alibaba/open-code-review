---
title: ChatGPT ile giriş önizlemesi
---

OpenCodeReview, OpenAI'nin herkese açık Responses API'si üzerinden uygun ChatGPT planlarının kullanım hakkından yararlanabilir. İsteğe bağlı bu önizleme açık kaynak CLI'de sunulur. OpenCodeReview bunun için ayrı abonelik ücreti almaz. Hesap, çalışma alanı, bölge ve model uygunluğunu OpenAI belirler.

`chatgpt` sağlayıcısı, belgelenmiş ChatGPT ile giriş akışını kullanır. `ocr auth login`, `status` ve `logout` için varsayılan sağlayıcı budur. Açıkça `--provider chatgpt` belirtmek de desteklenir.

OAuth, belgelenmiş OpenID Connect ve OpenAI kapsamlarının tamamını ister: `openid` kimliği doğrular; `profile` ve `email` hesap bilgilerini sağlar; `offline_access` siz yokken yenileme token'ıyla oturum yenilenmesine izin verir; `resource.invoke` kaynak çağrılarını yetkilendirir; `chatgpt.tokens.use.direct` ChatGPT plan kapasitesinin doğrudan kullanımına izin verir. İlk dinamik kayıt `agent_name_hint=OpenCodeReview` içerir. Akış bağlayıcı izni istemez ve Codex CLI istemci kaydını yeniden kullanmaz. Yalnızca bu belgelenmiş kapsamları ister; keyfî ek yetkiler istemez.

İstemci kaydı ilk giriş sırasında OpenAI tarafından verilir ve o hesap için saklanır. Uygulama ilk kayıt ipucunda gerçek adını gönderir, tüm kurulumlarda aynı adı kullanır ve sonraki girişlerde bu ipucunu göndermez. Her girişte yerel kuruluma ait sabit, anlam taşımayan bir `ext_agent_host_id` de gönderir. Aynı kurulum aynı ana makine kimliğini korur.

Yetkilendirmede yeni state, nonce ve PKCE değerleri kullanılır. Kod değişiminden önce geri çağırma state değeri kontrol edilir. Dönen kimliğe veya verilen kapsamlara güvenilmeden önce ID token imzası, sağlayıcının RSA JWKS anahtarlarıyla doğrulanır; issuer, audience, subject, geçerlilik süresi ve nonce kontrol edilir.

## Giriş yapma ve model seçme {#sign-in-and-select-a-model}

```bash
ocr auth login --provider chatgpt
ocr auth status --provider chatgpt
ocr llm models
ocr config set provider chatgpt
ocr config set providers.chatgpt.model gpt-6-luna
ocr config set providers.chatgpt.extra_body '{"reasoning":{"effort":"low"}}'
ocr llm test
```

Katalog komutu hesaba özgü önerileri ve görünen adları sunucu sırasıyla gösterir. Model yapılandırılmamışsa sağlayıcı listede görünür ilk modeli seçer. Yapılandırmada veya `--model` ile açıkça belirtilen model kimliği, katalogda bulunmasa bile değiştirilmeden OpenAI'ye gönderilir. Seçili hesabın o modeli kullanıp kullanamayacağını OpenAI belirler. OpenCodeReview bu sağlayıcı için derlenmiş model kısıtlamaları veya takma adlar içermez.

`extra_body` içindeki akıl yürütme çabası bir API parametresidir. Ayrı `--effort` inceleme seçeneği ise OpenCodeReview'un inceleme turu ve araç bütçelerini belirler.

Giriş, sistem tarayıcısını açar ve yalnızca `127.0.0.1` üzerinde dinler. Bunun yerine tek kullanımlık yerel URL yazdırmak için `--no-browser` kullanın. CLI, saklanmış ID token ipucu içeren bir yetkilendirme URL'sini hiçbir zaman yazdırmaz. Bu önizlemede cihaz koduyla kimlik doğrulama desteklenmez.

Önizleme, uygun ChatGPT hesabına sahip tek kişinin yerel kullanımı içindir. Ortak havuz veya barındırılan yeniden satış, kişisel kimlik bilgileriyle paylaşımlı CI ya da API faturalandırmasına alternatif yedek yol olarak kullanmayın. Hesap ve kullanım uygunluğunu OpenAI belirler; bu belge tasarruf veya erişilebilirlik garantisi vermez.

Giriş başarılı olur ancak ChatGPT planı izni verilmezse OpenCodeReview hesap kimliğini saklar ve plan kullanımının kapalı olduğunu bildirir. Etkin saklanmış kayıt için izin ekranını şu komutla isteyin:

```bash
ocr auth login --provider chatgpt --enable-plan
# For another retained registration:
ocr auth login --provider chatgpt --enable-plan --account <saved-client-id>
```

Bu açık işlem `prompt=consent`, istenen tüm kapsamlar ve kaynak ile kayıtlı istemci ve ana makine kimliklerini gönderir. `--new-account` ile birlikte kullanılamaz. Normal tekrar giriş, izin ekranını zorlamaz. Dönen izin plan kullanımını yetkilendirene kadar model çağrıları kapalı kalır. Bunun yerine API anahtarlı bir sağlayıcı yapılandırabilirsiniz; OpenCodeReview faturalandırma yollarını otomatik değiştirmez.

## Hesaplar ve çıkış {#accounts-and-sign-out}

```bash
ocr auth login --provider chatgpt --new-account
ocr auth status --provider chatgpt
ocr auth select <saved-client-id> --provider chatgpt
ocr auth login --provider chatgpt --account <saved-client-id>
ocr auth logout --provider chatgpt --account <saved-client-id>
```

Birden fazla kayıt aynı e-postayı paylaşsa bile verilen her istemci kimliği doğrulanmış issuer ve subject ile ilişkili kalır. Tekrar giriş aynı istemci kimliğini kullanır. Kod değişiminde `invalid_grant` alınırsa giriş, geri çağırmada verilen istemci kimliği ve yeni state, nonce, PKCE değerleriyle yetkilendirmeyi bir kez yeniden başlatır. Bekleyen kayıt, imzalı kimlik doğrulanmadan etkinleştirilmez. İkinci hata giriş işlemini durdurur ve önceki kimlik bilgilerini korur.

Kimlik bilgisi veritabanı `~/.opencodereview/auth/chatgpt/accounts.json` dosyasıdır. Ayrı `host.json` dosyası sabit kurulum kimliğini saklar. Kimlik bilgisi dosyaları atomik olarak değiştirilir.

Unix'te kimlik bilgisi dosyalarının izinleri yalnızca sahibine erişim veren `0600`, dizinin izinleri `0700` olur. Windows'ta erişim, yerel kullanıcı profilinden devralınan ACL'lere bağlıdır; OpenCodeReview DACL oluşturmaz veya doğrulamaz. Go'nun [`os.Chmod`](https://pkg.go.dev/os#Chmod) işlevi Windows'ta yalnızca salt okunur özniteliğini değiştirir. ACL'leri erişimi hesabınızla sınırlayan bir profil kullanın, girişten önce izinleri kontrol edin ve paylaşılan ya da geniş erişimli konumlardan kaçının. Dizinin kullanıcı profili altında olması tek başına gizlilik garantisi değildir.

Depo değişiklikleri ve token yenileme dönüşümü aynı işletim sistemi dosya kilidini kullanır. Diğer hesapların kullanılabilmesi için tarayıcı yetkilendirmesi kilit dışında yürütülür. Tamamlanırken depo yeniden yüklenir ve saklanan kaydın revizyonu kontrol edilir. O kayıt üzerinde çıkış, yenileme veya başka bir tamamlanmış giriş, bekleyen sonucu geçersiz kılar. İlgisiz hesap güncellemeleri korunur. Erişim token'ları, dönüşümlü yenileme token'larıyla süreleri dolmadan yenilenir.

Yenileme yeni token'lar döndürür ancak doğrulama iptal veya ID token doğrulaması için açık anahtarların alınmasındaki geçici hata nedeniyle kesilirse OCR alınan token'ları saklar. İlk komutu yeniden çalıştırın. CLI yeniden başlatılmış olsa bile sonraki istek doğrulamayı sürdürür.

Yenilenen ID token imza veya kimlik doğrulamasından geçemezse OCR yeniden denemeyi bırakır ve tekrar giriş ister. Hatada gösterilen komutu, kayıtlı istemci kimliğiyle çalıştırın:

```bash
ocr auth login --provider chatgpt --account <saved-client-id>
```

Başarılı giriş, reddedilen kimlik bilgilerinin yerini alır.

Çıkış, ID token doğrulaması bekleyen yeni token dâhil en son yenileme token'ıyla yenilenebilir oturumu iptal etmeyi dener. Erişim, yenileme ve ID token'larını temizler; hesap kaydını ve ana makine kimliğini korur. Çıkıştan sonraki girişte `id_token_hint` gönderilmez. Uzaktaki iptal doğrulanamazsa CLI yerel token'ların temizlendiğini bildirir ve ChatGPT ayarlarından uygulama bağlantısını kesmenizi ister.

## Aktarım ve kullanım {#transport-and-usage}

Bu sağlayıcı yalnızca `https://api.openai.com/v1/responses` adresine gönderir. Uç nokta, protokol ve kimlik bilgisi geçersiz kılmaları reddedilir. `store: false` ile akış kullanır, tüm geçmişi gönderir, yerel işlevleri `ocr` ad alanında gruplar ve sonraki turlar için çağrı kimliklerini ve şifreli akıl yürütmeyi korur. Yalnızca `response.completed` başarı sayılır. Başarısız, eksik veya kesilmiş akışlar; çıktı başladıktan sonra gelen kullanım sınırı hataları dâhil isteği durdurur.

Önizleme istek alanları OpenAI'nin desteklediği alt kümeyle sınırlıdır. `extra_body`, `reasoning`, `text` ve `prompt_cache_key` kabul eder; model, geçmiş, araçlar, akış modu veya saklama politikasının yerini alamaz. Sağlayıcının ham kullanım bilgisi yanıtta bulunur. Mevcut isteğe bağlı ham inceleme kaydı, kimlik bilgisi başlıkları olmadan akışı kaydeder.

**ChatGPT planının kullanımı:** Uygun istekler seçili hesabın ChatGPT planını kullanır. [ChatGPT ayarlarından kullanımı yönetin](https://chatgpt.com/settings/usage). Kullanım sınırı hatası plan genelindeki veya uygulamaya özgü bir sınırı yansıtabilir; CLI sıfırlanma zamanı tahmin etmez.

Güncel gereksinimler için OpenAI'nin [SIWC açık kaynak genel bakışına](https://developers.openai.com/siwc/token-sharing-open-source), [giriş sözleşmesine](https://developers.openai.com/siwc/token-sharing-open-source/sign-in) ve [önizleme sınırlamalarına](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations) bakın.

`ocr auth` ve `ocr llm models` için [CLI başvurusu](../cli-reference/), sağlayıcı ayarları için [Yapılandırma](../configuration/) sayfasına bakın.

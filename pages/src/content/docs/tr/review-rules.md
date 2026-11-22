---
title: İnceleme kuralları
sidebar:
  order: 7
---

Kurallar, OCR'ın her dosyayı incelerken **nelere odaklanacağını** belirler. Üç katmandaki JSON dosyalarında tanımlanırlar; bunlara çalıştırılabilir dosyayla gelen yerleşik sistem kuralları eklenir.

## Öncelik sırası {#priority-chain}

OCR, kuralları **dört katmanlı bir öncelik sırasına** göre çözümler. Her dosya yolu için katmanlar sırayla denenir; ilk eşleşen kalıp kullanılır.

| Öncelik | Kaynak | Yol | Açıklama |
| --- | --- | --- | --- |
| 1 (en yüksek) | `--rule` seçeneği | Kullanıcının belirttiği yol | Komut satırından verilen kurallar önceliklidir. |
| 2 | Proje yapılandırması | `<repoDir>/.opencodereview/rule.json` | Projeye özgü kurallar; depoya eklenebilir. |
| 3 | Genel yapılandırma | `~/.opencodereview/rule.json` | Kullanıcının tüm projeleri için tercihleri. |
| 4 (en düşük) | Sistem varsayılanı | Gömülü `system_rules.json` | Yaygın diller için yerleşik kurallar. |

Daha yüksek öncelikli katmanın dosyası yoksa hata verilmeden atlanır. `.opencodereview/rule.json` eklenmemiş bir projede genel ve sistem katmanları kullanılır. Sistem katmanı çalıştırılabilir dosyaya gömülü olduğundan **her zaman** vardır; dolayısıyla her zaman bir kural çözümlenir.

## Kural dosyası biçimi (1–3. katmanlar) {#rule-file-format-layers-1-3}

```json
{
  "include": ["src/**/*.{ts,tsx}", "src/**/*.go"],
  "exclude": ["**/*.test.ts", "**/generated/**"],
  "rules": [
    {
      "path": "src/api/**/*.go",
      "rule": "All exported handlers must validate request bodies before use."
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks, parameter errors, and missing closing tags."
    }
  ]
}
```

Birbirinden bağımsız üç alan vardır:

- `include` — İsteğe bağlıdır. Yerleşik varsayılan dışlama kalıplarını (aşağıdaki test dosyası dışlamaları gibi) **atlatan** glob kalıplarıdır. İzin verilen dosyaların tamamını belirleyen bir liste değildir: eşleşmeyen dosyalar da `unsupported_ext` ve `default_path` kontrollerinden geçerek incelemeye alınabilir.
- `exclude` — İsteğe bağlıdır. OCR'ın **incelememesi** gereken dosyaları belirler. Kullanıcının tanımladığı filtreler arasında en yüksek önceliğe sahiptir.
- `rules` — **Tanımlandıkları sırayla** değerlendirilen `{path, rule}` girdileri dizisidir. Dosyayla ilk eşleşen `path` kalıbı, OCR'ın o dosya için modele göndereceği istemi belirler.

Her `rules` girdisi isteğe bağlı üçüncü bir alan da kabul eder:

- `merge_system_rule` — Varsayılanı `false` değeridir. `false` olduğunda eşleşen girdi dosyanın yerleşik sistem kuralının **yerini alır**. `true` olduğunda sistem kuralı korunur ve kullanıcı kuralıyla **birleştirilir**.

```json
{
  "rules": [
    {
      "path": "**/*",
      "rule": "Security review: flag hardcoded secrets, unvalidated redirects, and missing authz checks.",
      "merge_system_rule": true
    }
  ]
}
```

Bu girdiyle `ocr rules check src/main/java/com/example/UserService.java` komutu her iki bölümü de gösterir:

```
$ ocr rules check src/main/java/com/example/UserService.java
Source: Project (.opencodereview/rule.json)
Pattern: **/*
Rule:
────────────────────────────────────────
## System-Specific Rules (Mandatory)

…contents of java.md…

---

## User-Specific Rules (Mandatory)

Security review: flag hardcoded secrets, unvalidated redirects, and missing authz checks.
────────────────────────────────────────
```

Birleştirmeyle ilgili iki ayrıntı:

- **Sistem bölümü her dosya için ayrı çözümlenir.** Yukarıdaki girdi tüm dosyaları kapsasa da `.java` dosyası `java.md`, `.py` dosyası `python.md`, tanınmayan uzantı ise `default.md` kullanır. Böylece her uzantı için aynı kuralı tekrarlamadan ilgili dilin kurallarına kendi kuralınızı ekleyebilirsiniz.
- Bölümlerden biri boş olabilir. Sistem katmanı bir dosya için içerik döndürmezse yalnızca sizin kuralınız, sizin kural metniniz boşsa yalnızca sistem kuralı kullanılır. Boş bölümün yerine yer tutucu eklenmez.

Alan, üç kullanıcı katmanından da okunur: `--rule`, projenin `.opencodereview/rule.json` dosyası ve `~/.opencodereview/rule.json`. Katman önceliği değişmez: ilk eşleşen girdi kullanılır ve eşleşen katman daha düşük öncelikli katmanları devre dışı bırakır.

Birleştirme yalnızca **sistem** katmanına uygulanır. Kendi girdilerinizden ikisi aynı dosyayla eşleşirse ilk girdi kullanılır; `merge_system_rule` bunları birbiriyle birleştirmez.

### Glob özellikleri {#glob-features}

OCR, eşleştirme için [`bmatcuk/doublestar/v4`](https://pkg.go.dev/github.com/bmatcuk/doublestar/v4) kullanır:

- `*` — `/` dışındaki karakterlerle eşleşir.
- `**` — Dizin sınırlarını aşar; `src/**/*.go` her derinliği kapsar.
- `{a,b,c}` — Süslü parantez açılımı. `*.{ts,tsx,js,jsx}` dört kalıba açılır ve sırayla denenir.
- `?` — Tek karakterle eşleşir.
- `[abc]` — Karakter sınıfıdır.

> Eşleştirme **büyük/küçük harfe duyarsızdır**; dosya yolu önce küçük harfe dönüştürülür. Emin değilseniz `ocr rules check <path>` ile doğrulayın.

## Dosyalar nasıl filtrelenir? {#how-files-are-filtered}

Filtre, [`internal/agent/selection.go`](https://github.com/alibaba/open-code-review/blob/main/internal/agent/selection.go) içindeki altı aşamalı algoritmadır. OCR her fark için şu kontrolleri yapar:

1. **`binary`** — Dosya ikili dosyaysa dışlanır.
2. **`secret_exclude`** — Eski veya yeni yol, yerleşik gizli dosya korumasıyla eşleşiyorsa dışlanır. Bu koruma kullanıcı kurallarından önce uygulanır ve `include` ile geçersiz kılınamaz. Kalıplar aşağıdaki [Yerleşik gizli dosya yolları](#built-in-secret-paths) bölümündedir.
3. **`user_exclude`** — Yol, kullanıcının `exclude` kalıplarından biriyle eşleşiyorsa dışlanır.
4. **`user_include`** — Yol, tanımlanmış bir `include` kalıbıyla eşleşiyorsa **hemen tutulur**; aşağıdaki `unsupported_ext` ve `default_path` aşamaları atlanır.
5. **`unsupported_ext`** — Dosya uzantısı [izin listesinde](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/supported_file_types.json) yoksa dışlanır.
6. **`default_path`** — Yol, yerleşik dışlama kalıbıyla eşleşiyorsa dışlanır. Bunlar test dosyalarını (`**/*_test.go`, `**/*.test.{js,jsx,ts,tsx}`, `**/*_spec.rb` vb.) ve bağımlılık/derleme çıktısı dizinlerini (`**/node_modules/**`, `**/vendor/**`, `**/target/**` vb.) kapsar.

Altı aşamayı geçen dosyalar LLM'e gönderilir; ancak farkın kendisi `max_tokens` değerinin %80'ini aşıyorsa `selectFiles`, dosyayı `too_large` olarak dışlar. Yeni yolu `/dev/null` olan dosyalar da incelenecek yeni içerik olmadığından `deleted` olarak işaretlenir. Token harcamadan filtre sonucunu görmek için `ocr review --preview` kullanın.

### Yerleşik gizli dosya yolları {#built-in-secret-paths}

Yerleşik gizli dosya yolları incelenmez ([`internal/config/allowlist/default_secret_patterns.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/default_secret_patterns.json)):

- `**/.ssh/**`
- `**/id_rsa`
- `**/id_dsa`
- `**/id_ecdsa`
- `**/id_ed25519`
- `**/.netrc`
- `**/_netrc`
- `**/.npmrc`
- `**/.pypirc`
- `**/.dockercfg`

`.env` ve tüm `.env.*` çeşitleri de gizli dosya yolu sayılır. `.env.example`, `.env.sample` ve `.env.template` şablonları bunun dışındadır.

### Varsayılan yol dışlamaları {#default-path-exclusions}

[Yerleşik dışlama listesi](https://github.com/alibaba/open-code-review/blob/main/internal/config/allowlist/default_exclude_patterns.json) iki grubu kapsar. Farklı dillerdeki test dosyaları, test verileri, anlık görüntüler ve üretilmiş kod:

- `**/*_test.go`
- `**/src/test/java/**/*.java`
- `**/src/test/**/*.{kt,kts}`
- `**/*Test.fs`
- `**/*.test.{js,jsx,ts,tsx}`
- `**/*.spec.{js,jsx,ts,tsx}`
- `**/__tests__/**`
- `**/test/**/*_test.py`
- `**/tests/**/*_test.py`
- `**/*_test.py`
- `**/test_*.py`
- `**/*_spec.rb`
- `**/spec/**/*_spec.rb`
- `**/*Test.java`
- `**/*Tests.java`
- `**/*_test.rs`
- `**/*_tests.rs`
- `**/tests.rs`
- `**/tests/**/*.rs`
- `**/benches/**/*.rs`
- `**/oh_modules/**`
- `**/*.test.ets`
- `**/test/**/*.jl`
- `**/test/**/*.hs`
- `**/*Spec.hs`
- `**/test/**/*.lhs`
- `**/*Spec.lhs`
- `**/tests/**/*.nim`
- `**/tests/**/*.nu`
- `**/test/**/*.nu`
- `**/*_test.nu`
- `**/test_*.nu`
- `**/test-*.nu`
- `**/tests/**/*.R`
- `**/__snapshots__/**`
- `**/*.snap`
- `**/testdata/**`
- `**/fixtures/**`
- `**/.ipynb_checkpoints/**`
- `**/*.generated.*`
- `**/*.gen.go`
- `**/*.pb.go`
- `**/*.pb.cc`
- `**/*.pb.h`
- `**/*Test.swift`
- `**/*Tests.swift`
- `**/Tests/**/*.swift`
- `**/tests/**/*.elm`
- `**/vendor/**/*.{jsonnet,libsonnet}`
- `**/test/**/*.zig`
- `**/*_test.zig`
- `**/kitex_gen/**/*.go`
- `**/*.capnp.h`
- `**/*.capnp.go`
- `**/*.capnp.ts`
- `**/*_capnp.rs`
- `**/*_capnp.py`
- `**/test/**/*.ml`
- `**/tb_*.{v,sv,vhd,vhdl}`
- `**/*_tb.{v,sv,vhd,vhdl}`
- `lib/**/*.sol`
- `**/*.t.sol`
- `**/test/**/*.sol`
- `**/tests/**/*.sol`
- `**/test/**/*.vy`
- `**/tests/**/*.vy`

Bağımlılık ve derleme çıktısı dizinleri:

- `**/node_modules/**`
- `**/bower_components/**`
- `**/vendor/**`
- `**/target/**`
- `**/dist/**`
- `**/__pycache__/**`, `**/.venv/**`, `**/site-packages/**`
- `**/Pods/**`, `**/Carthage/**`
- `**/.next/**`, `**/.nuxt/**`, `**/.gradle/**`, `**/.terraform/**`, …

`**/build/**` ve `**/bin/**` özellikle dışlanmaz; projeler bu dizinlerde elle yazılmış kaynak dosyalar da bulundurur.

Aynı dizinler, daha erken bir aşamada [`internal/diff/git.go`](https://github.com/alibaba/open-code-review/blob/main/internal/diff/git.go) içinde fark düzeyinde de filtrelenir. Bu liste yol önekiyle eşleşir, dolayısıyla yalnızca **depo kökündeki** dizinleri yakalar. `vendor/pkg/x.go`, dosya filtresine ulaşmadan `provider_directory` olarak bildirilir. `api/vendor/pkg/x.go` ise dosya filtresine ulaşır ve `default_path` olarak bildirilir. Yalnızca ikinci dosya `include` kuralıyla geri alınabilir.

Bu kalıplardan biriyle eşleşen dosyayı **incelemek** için kullanıcı `include` listesine ekleyin; bu, varsayılan yol filtresini geçersiz kılar.

## Dosya bazında kural çözümleme {#rule-resolution-per-file}

Bir dosyanın inceleneceği belirlendikten sonra OCR, ajanın izleyeceği kural metnini seçer:

1. `--rule` (özel) katmanını tanım sırasıyla dener.
2. `<repo>/.opencodereview/rule.json` dosyasını tanım sırasıyla dener.
3. `~/.opencodereview/rule.json` dosyasını tanım sırasıyla dener.
4. Gömülü sistem kurallarına başvurur.

Gömülü `system_rules.json` içindeki tüm kalıplar, eşleşme sıraları korunarak aşağıda verilmiştir:

| Kalıp                                         | Kural belgesi                                                                                              |
| ----------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `**/*.properties`                               | `properties.md` — i18n / yapılandırma dosyaları.                                                         |
| `**/*{mapper,dao}*.xml`                         | `mapper_dao_xml.md` — MyBatis biçiminde eşleyici SQL.                                                       |
| `**/pom.xml`                                    | `pom_xml.md` — Maven bağımlılıkları.                                                                    |
| `**/build.gradle`                               | `build_gradle.md` — Gradle bağımlılıkları.                                                              |
| `**/package.json`                               | `package_json.md` — NPM bağımlılıkları / betikleri.                                                       |
| `**/Cargo.toml`                                 | `cargo_toml.md` — Rust bildirimi.                                                                      |
| `**/Move.toml` | `move_toml.md` — Move paket bildirimi. |
| `**/*.move` | `move.md` — Move kaynak kodu. |
| `**/composer.json`                              | `composer_json.md` — Composer bağımlılıkları, otomatik yükleme, betikler, eklentiler ve paket yapılandırması. |
| `**/*.{json,json5,jsonc}`                       | `json.md` — genel JSON (`.json5` ve `.jsonc` ile de eşleşir). |
| `.github/workflows/**/*.{yaml,yml}`             | `github_workflows.md` — GitHub Actions iş akışı YAML dosyaları.                                                 |
| `.github/**/*.{yaml,yml}`                       | `github_config.md` — diğer `.github` yapılandırma YAML dosyaları.                                                     |
| `**/*.{yaml,yml}`                               | `yaml.md`                                                                                             |
| `**/*.java`                                     | `java.md`                                                                                             |
| `**/*.go`                                       | `go.md` — Go kaynak kodu.                                                                                  |
| `**/*.{ftl,ftlh,ftlx}`                          | `freemarker.md` — FreeMarker şablonları (SSTI / XSS / null işleme).                                  |
| `**/*.{hbs,mustache}`                           | `handlebars_mustache.md` — Handlebars ve Mustache şablonları.                                         |
| `**/*.{jinja2,j2}`                              | `jinja.md` — Jinja şablonları                                                                          |
| `**/*.pug`                          | `pug.md` — Pug şablonları.                                                                             |
| `**/*.{ejs,liquid,njk}`                         | `ejs_liquid_njk.md` — EJS, Liquid ve Nunjucks şablonları (SSTI / XSS / kaçışlama). |
| `**/*.{haml,slim}`                              | `haml_slim.md` — HAML ve Slim şablonları (SSTI / XSS / kaçışlama / girinti anlamı). |
| `**/*.erb`                                      | `erb.md` — ERB şablonları (SSTI / XSS / kaçışlama / boşluk ve çıktı yapısı). |
| `**/*.ets`                                      | `arkts.md` — ArkTS / HarmonyOS.                                                                       |
| `**/*.astro`                                    | `astro.md` — Astro bileşenleri ve adaları.                                                            |
| `**/*.{ts,js,tsx,jsx,mjs,cjs,mts,cts}`          | `ts_js_tsx_jsx.md`                                                                                    |
| `**/*.{kt,kts}`                                 | `kotlin.md`                                                                                           |
| `**/*.scala`                                    | `scala.md` |
| `**/*.{fs,fsi,fsx}`                             | `fsharp.md` — F# uygulama, imza ve betik dosyaları.                                         |
| `**/*.cs`                                       | `csharp.md` — C# kaynak dosyaları. |
| `**/*.rs`                                       | `rust.md`                                                                                             |
| `**/*.{cpp,cc,cxx,hpp,hxx}`                     | `cpp.md`                                                                                              |
| `**/*.c`                                        | `c.md`                                                                                                |
| `**/*.{cu,cuh}`                                 | `cuda_hip.md` — CUDA çekirdekleri ve başlık dosyaları. |
| `**/*.hip`                                      | `cuda_hip.md` — HIP (AMD ROCm) çekirdekleri. |
| `**/*.{py,pyi,ipynb}`                           | `python.md` — Python kaynak kodu.                                                                          |
| `**/*.{rb,rake}`, `**/*.gemspec`, `**/{Gemfile,Rakefile}` | `ruby.md` — Ruby kaynak kodu, Rake görevleri, gemspec, Gemfile ve Rakefile dosyaları. |
| `**/*.{php,phtml}`                              | `php.md` — PHP kaynak kodu ve şablonları.                                                              |
| `**/*.proto`                                    | `protobuf.md` — Protocol Buffers aktarım biçimi uyumluluğu.                                                  |
| `**/*.po`                                       | `po.md` — gettext çeviri kaynak katalogları.                                                        |
| `**/*.pot`                                      | `pot.md` — gettext şablon dosyaları.                                                                    |
| `**/*.{graphql,gql}`                            | `graphql.md` — GraphQL şeması ve işlemleri.                                                         |
| `**/*.prisma`                                   | `prisma.md` — Prisma şeması.                                                                          |
| `**/*.jl`                                       | `julia.md` — Julia kaynak kodu.                                                                            |
| `**/*.R`                                        | `r.md`                                                                                                |
| `**/*.{tf,tofu,hcl,tfvars}`                     | `terraform.md` — Terraform / HCL.                                                                     |
| `**/*.bicep`                                    | `bicep.md` — Bicep (Azure) şablonları.                                                                 |
| `**/*.dhall`                                    | `dhall.md` — Dhall yapılandırma dili. |
| `**/*.nix`                          | `nix.md` — Nix ifadeleri ve flake dosyaları.                                                                |
| `**/*.{hs,lhs}`                     | `haskell.md` — Haskell kaynak kodu.                                                                        |
| `**/*.{nim,nims,nimble}`            | `nim.md` — Nim kaynak kodu ve paket bildirimleri.                                                          |
| `**/*.swift`                        | `swift.md` — Swift kaynak kodu.                                                                            |
| `**/*.nu`                                       | `nushell.md` — Nushell betikleri.                                                                       |
| `**/*.elm`                                      | `elm.md` - Elm kaynak kodu.                                                                                |
| `**/*.{jsonnet,libsonnet}`                      | `jsonnet.md` — Jsonnet yapılandırma şablonları ve kitaplıkları.                                         |
| `**/*.zig`                          | `zig.md` — Zig kaynak kodu.                                                                                |
| `**/*.thrift`                                   | `thrift.md` — Apache Thrift IDL aktarım biçimi uyumluluğu.                                                   |
| `**/*.{bzl,bazel,star}`, `**/{BUILD,WORKSPACE}` | `starlark.md` — Starlark ve Bazel derleme, makro ve kural tasarımı.                                     |
| `**/*.capnp`                                    | `capnp.md` — Cap'n Proto şeması aktarım biçimi uyumluluğu.                                                   |
| `**/*.{ml,mli}`                     | `ocaml.md` — OCaml kaynak kodu.                                                                            |
| `**/*.{re,rei}`                     | `ocaml.md` — ReasonML kaynak kodu.                                                                         |
| `**/*.{v,sv,vh}`                                | `verilog.md` — Verilog ve SystemVerilog RTL.                                                         |
| `**/*.{vhd,vhdl}`                               | `vhdl.md` — VHDL RTL.                                                                                 |
| `**/*.m`                                        | `matlab.md` (veya [içerikten dil belirleme](#content-sniffing-for-m-files) ile `objc.md`)                      |
| `**/*.mm`                                       | `objc.md` — Objective-C++ kaynak kodu.                                                                     |
| `**/*.sol`                                      | `solidity.md` — Solidity akıllı sözleşmeleri.                                                             |
| `**/*.vy`                                       | `vyper.md` — Vyper akıllı sözleşmeleri.                                                                   |
| `**/*.rego`                                     | `rego.md` — Rego ilkeleri (OPA).                                                                        |
| `**/*.{glsl,hlsl,wgsl,metal}`                   | `shader.md` — GLSL, HLSL, WGSL ve Metal gölgelendiricileri.                                                    |
| `**/*.{ps1,psm1,psd1}`                          | `powershell.md` — PowerShell betikleri, modülleri ve veri/bildirim dosyaları.                               |
| `**/*.{clj,cljs,cljc,edn}`                      | `clojure.md` — Clojure / ClojureScript kaynak kodu ve EDN verileri. |
| `**/*.sql`                                      | `sql.md` — SQL geçiş betikleri ve sorguları. |
| _(varsayılan)_                                    | `default.md`                                                                                          |

Çözümlenen kural metni, planlama ve ana görev istemlerindeki `{{system_rule}}` yer tutucusuna yerleştirilir.

### `.m` dosyalarında içerikten dil belirleme {#content-sniffing-for-m-files}

`.m` uzantısı MATLAB ve Objective-C tarafından paylaşılır. OCR, dosyanın boş olmayan ilk satırına bakar. Objective-C'ye benziyorsa (ör. `#import`, `@implementation` veya C biçiminde yorum) `matlab.md` yerine `objc.md` kullanılır. İçerik okunamazsa `matlab.md` kullanılır.

> **Kararlılık notu:** Bu sezgisel yöntem OCR sürümleri arasında değişebilir. `.m` dosyaları için kesin bir seçim istiyorsanız proje düzeyinde açık bir kural tanımlayın; proje kuralları sistem katmanından her zaman önceliklidir.

## Hangi kuralın seçildiğini görme: `ocr rules check` {#inspecting-which-rule-wins-ocr-rules-check}

```bash
$ ocr rules check src/main/java/com/example/UserService.java
File: src/main/java/com/example/UserService.java
Source: System built-in
Pattern: **/*.java
Rule:
────────────────────────────────────────
…contents of java.md…
────────────────────────────────────────
```

```bash
$ ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml
File: src/main/resources/mapper/UserMapper.xml
Source: Custom (--rule)
Pattern: **/*mapper*.xml
Rule:
────────────────────────────────────────
…contents of your custom rule…
────────────────────────────────────────
```

Bir kural beklediğiniz gibi çalışmıyorsa bu komutu kullanın. Seçilen **katmanı** ve **kalıbı** gösterir.

## Kullanım örnekleri {#recipes}

### Proje düzeyinde kodlama standardı uygulama {#project-level-enforce-a-coding-standard}

`<repo>/.opencodereview/rule.json` olarak kaydedip depoya ekleyin:

```json
{
  "rules": [
    {
      "path": "src/api/**/*.go",
      "rule": "Every public handler must `defer tx.Rollback()` immediately after starting a transaction."
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks, missing parameter binding, and unclosed XML tags."
    }
  ]
}
```

### Proje düzeyinde üretilmiş kodu atlama ve src dizinine odaklanma {#project-level-skip-generated-code-focus-on-src}

```json
{
  "include": ["src/**/*.{ts,tsx,js,jsx}"],
  "exclude": ["**/*.gen.ts", "**/generated/**"]
}
```

`include` tanımlandığında `src/` içindeki dosyalar, yerleşik dışlama kalıbıyla (ör. test dosyası kalıbıyla) eşleşseler bile tutulur. `src/` dışındaki dosyalara normal uzantı ve varsayılan yol kontrolleri uygulanmaya devam eder. `include`, yalnızca bu dosyaların inceleneceğini belirten bir liste değil, filtreleri atlama yoludur.

### PR'a özel kural kullanma {#per-pr-override}

```bash
ocr review --rule ./.review-rules-only-for-this-pr.json
```

Proje ve genel katmanları atlar. Tek bir PR için farklı bir kontrol listesi gerektiğinde (ör. yalnızca güvenlik incelemesi) kullanışlıdır.

### Genel kişisel tercihler {#global-personal-preferences}

Makinenizdeki tüm depoların kullanması için `~/.opencodereview/rule.json` dosyasına yazın:

```json
{
  "rules": [
    {
      "path": "**/*.{ts,tsx,js,jsx}",
      "rule": "Always check for unhandled promise rejections; warn on `// eslint-disable` without a reason comment."
    }
  ]
}
```

### Yerleşik dil kurallarına genel güvenlik kuralları ekleme {#global-security-rules-on-top-of-the-built-in-language-rules}

Tüm dosyalarla eşleşen bir kullanıcı kuralı, varsayılan olarak dil kurallarının **yerini alır**. Dolayısıyla yalnızca `**/*` girdisi eklemek `java.md`, `python.md` ve diğer dil kurallarını devreden çıkarır. İkisini de korumak için `merge_system_rule` kullanın:

```json
{
  "rules": [
    {
      "path": "**/*",
      "rule": "Security review: flag hardcoded secrets, unvalidated redirects, and missing authz checks.",
      "merge_system_rule": true
    }
  ]
}
```

Tüm depolara uygulamak için `~/.opencodereview/rule.json`, tek depoya uygulamak için `<repo>/.opencodereview/rule.json` konumuna kaydedin. Sistem bölümü her dosya için ayrı çözümlendiğinden her dil kendi yerleşik kurallarını sizin kurallarınızla birlikte kullanır; uzantıları tek tek sıralamanız gerekmez.

Genel dosya, üç kullanıcı katmanının **en düşük öncelikli** olanıdır. `--rule` veya proje dosyası aynı dosyayla eşleşen bir girdi içeriyorsa o girdi kullanılır ve genel kurala ulaşılmaz. Bu nedenle tüm dosyaları kapsayan kuralı yalnızca bir konuma yerleştirin.

## Ayrıca bkz. {#see-also}

- [Komut satırı başvurusu](../cli-reference/) — `ocr review --rule`, `--preview` ve `ocr rules check`.
- [Yapılandırma](../configuration/) — Dosya konumları ve katmanlı çözümleme sırası.
- [Mimari](../architecture/) — Çözümlenen kuralın ajan isteminde kullanımı.

# Đóng góp cho OpenCodeReview

Cảm ơn bạn đã quan tâm đến việc đóng góp cho OpenCodeReview! Mọi đóng góp đều quan trọng — dù là sửa lỗi chính tả, báo cáo lỗi, hay triển khai một tính năng mới.

[English Version](../../CONTRIBUTING.md) | [简体中文版](CONTRIBUTING.zh-CN.md) | [日本語版](CONTRIBUTING.ja-JP.md) | [한국어](CONTRIBUTING.ko-KR.md) | [Русский](CONTRIBUTING.ru-RU.md) | [Türkçe](CONTRIBUTING.tr-TR.md)

## Quy tắc ứng xử

Khi tham gia dự án này, bạn đồng ý duy trì một môi trường tôn trọng và hòa nhập. Hãy tử tế và mang tính xây dựng trong mọi tương tác.

## Các cách đóng góp

Có nhiều cách đóng góp ngoài việc viết mã:

- **Báo cáo lỗi** — Tìm thấy thứ gì đó bị hỏng? Hãy mở một issue kèm các bước tái hiện.
- **Đề xuất tính năng** — Có ý tưởng cải tiến? Bạn có thể bắt đầu một cuộc trò chuyện trong [GitHub Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas) hoặc mở một issue [Feature Request](https://github.com/alibaba/open-code-review/issues/new?template=feature_request.yml).
- **Cải thiện tài liệu** — Sửa lỗi chính tả, làm rõ giải thích, hoặc thêm ví dụ. Bạn cũng có thể mở một [Documentation Issue](https://github.com/alibaba/open-code-review/issues/new?template=docs_report.yml) để báo cáo vấn đề.
- **Rà soát pull request** — Giúp chúng tôi rà soát mã từ những người đóng góp khác.
- **Viết mã** — Sửa lỗi, thêm tính năng, hoặc cải thiện hiệu năng.

## Bắt đầu

### Yêu cầu tiên quyết

- [Go 1.26.10+](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### Thiết lập

```bash
# 1. Fork the repository on GitHub

# 2. Clone your fork
git clone https://github.com/<your-username>/open-code-review.git
cd open-code-review

# 3. Add upstream remote (for syncing updates from the main repo)
git remote add upstream https://github.com/alibaba/open-code-review.git

# 4. Build the project
make build

# 5. Run tests
make test
```

Nếu mọi thứ đều đạt, bạn đã sẵn sàng đóng góp.

> **Lưu ý:** Remote `upstream` là chỉ đọc đối với người đóng góp — nó được dùng để kéo các thay đổi mới nhất từ kho chứa chính. Bạn không thể push trực tiếp lên upstream. Mọi commit phải được push lên fork của bạn (`origin`) và gửi qua Pull Request.

### Kết thúc dòng (Line Endings)

Dự án này bắt buộc dùng kết thúc dòng LF thông qua `.gitattributes`. Hãy cấu hình Git để tự động chuẩn hóa kết thúc dòng:

```bash
git config core.autocrlf input
```

Điều này đảm bảo mọi CRLF đều được chuyển thành LF khi commit, ngăn ngừa các vấn đề về kết thúc dòng trong CI.

## Quy trình phát triển

### Phân nhánh

Tạo một nhánh tính năng từ `main`:

```bash
git checkout main
git pull upstream main
git checkout -b feat/your-feature-name
```

Sử dụng tiền tố để chỉ loại thay đổi:

| Tiền tố     | Mục đích                              |
| ----------- | ------------------------------------- |
| `feat/`     | Tính năng mới                         |
| `fix/`      | Sửa lỗi                               |
| `docs/`     | Chỉ tài liệu                          |
| `refactor/` | Tái cấu trúc mã (không đổi hành vi)   |
| `test/`     | Thêm hoặc cập nhật kiểm thử           |
| `chore/`    | Thay đổi về build, CI, hoặc công cụ   |

### Thông điệp commit

Tuân theo định dạng [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short summary>

[optional body]
```

Ví dụ:

```
feat(viewer): add session search
fix(llm): handle timeout errors in Anthropic API calls
docs(README): update configuration examples
```

`PR Commit Policy` kiểm tra tiêu đề PR và toàn bộ thông điệp của mọi commit, bao
gồm cả phần thân, không phân biệt chữ hoa chữ thường. Không được bao gồm `[skip ci]`, `[ci skip]`, `[no ci]`,
`[skip actions]`, `[actions skip]`, hoặc một dòng `skip-checks: true` / `skip-checks:true`.

Để sửa các lỗi này, hãy chỉnh sửa tiêu đề PR hoặc viết lại các thông điệp commit bị ảnh hưởng trên
nhánh đóng góp của bạn. Thêm một commit sạch không loại bỏ được các vi phạm trước đó.
Mô tả PR có thể thảo luận về các hướng dẫn này và không bị kiểm tra.

### Tiêu đề giấy phép (License Headers)

Mọi tệp nguồn (`.go`, `.js`, `.mjs`, `.ts`, `.tsx`, `.kt`, `.kts`, `.sh`, `.py`, `.css`) phải bao gồm một tiêu đề giấy phép SPDX. Sau khi tạo tệp mới, hãy chạy:

```bash
make license-add
```

Lệnh này tự động thêm tiêu đề yêu cầu. CI sẽ từ chối các PR thiếu tiêu đề.

### Chất lượng mã

Trước khi gửi thay đổi, hãy đảm bảo chúng vượt qua mọi kiểm tra:

```bash
# Format, lint, and verify license headers
make check

# Run tests with race detection
make test

# Build successfully
make build
```

### Cấu trúc dự án

```
├── acp/                  # ACP adapter for the CLI
├── bin/                  # NPM wrapper
├── cmd/opencodereview/   # CLI entry point
├── docs/                 # Localized README and CONTRIBUTING
├── examples/             # CI/CD integration examples
├── extensions/           # Editor extensions (VS Code and IDEA)
├── imgs/                 # Logo, benchmark, and highlight images
├── internal/
│   ├── agent/            # Review agent logic
│   ├── chatgptauth/      # ChatGPT OAuth and credential management
│   ├── config/           # Configuration management
│   ├── delegate/         # Delegation-mode review specs (no LLM)
│   ├── diff/             # Git diff parsing
│   ├── gitcmd/           # Bounded git subprocess runner
│   ├── llm/              # LLM API client (Anthropic, Bedrock & OpenAI)
│   ├── llmloop/          # Shared per-subtask tool-use loop
│   ├── mcp/              # MCP server client and tool adapter
│   ├── model/            # Data models
│   ├── pathutil/         # Canonical paths and containment checks
│   ├── release/          # Release asset URL naming checks
│   ├── scan/             # Full-file code review (ocr scan)
│   ├── session/          # Review session management
│   ├── stdout/           # Swappable stdout writer
│   ├── suggestdiff/      # Line-level diffs for suggestion rendering
│   ├── telemetry/        # OpenTelemetry integration
│   ├── tool/             # Built-in tools (file_read, code_search, etc.)
│   └── viewer/           # WebUI session viewer
├── npm/                  # Per-platform optional binary packages
├── pages/                # Landing page
├── plugins/              # Coding agent plugins
├── scripts/              # Install, publish, and check scripts
└── skills/               # Agent skill manifests
```

## Phát triển với sự hỗ trợ của AI

Việc sử dụng AI để hỗ trợ phát triển của bạn hoàn toàn được hoan nghênh, và chúng tôi vui nếu nó giúp việc đóng góp trở nên dễ dàng hơn. Điều chúng tôi không thể chấp nhận là mã do một mô hình tạo ra đi thẳng vào một commit mà không được đọc, với không phần thừa hay lỗi nào của nó được dọn dẹp. Những bản gửi như vậy làm chậm cuộc trò chuyện rà soát và khiến một pull request khó tiến triển hơn.

Nếu AI đóng một vai trò trong cách bạn làm việc, vui lòng tuân theo các quy tắc dưới đây.

### Quy tắc:

1. **Bạn phải công khai trong issue hoặc pull request đầu tiên của mình rằng bạn đã sử dụng AI/LLM, cũng như các công cụ/mô hình bạn đã dùng.**
2. Bạn nên hiểu từng dòng mã do AI viết và biết AI đã làm gì.
3. Khi một người rà soát hỏi về lý do của một thay đổi, bạn phải tự giải thích được, bất kể bạn hay AI đã viết nó. Nội dung các câu trả lời của bạn cho câu hỏi của maintainer và các bình luận rà soát phải đến từ sự hiểu biết của chính bạn — bạn chỉ có thể dùng AI/LLM để dịch hoặc trau chuốt câu chữ, không được để nó tạo ra câu trả lời thay bạn.
4. PR của bạn không nên chứa các chu kỳ lặp lại như `AI generated -> fixed -> fixed -> fixed`. Điều này có thể cho thấy bạn đã không rà soát mã do AI tạo ra, mà thay vào đó cứ để AI sửa vấn đề mỗi khi chúng phát sinh, lặp đi lặp lại.
5. Bạn phải tự rà soát mọi mã, văn bản và nội dung khác do AI/LLM tạo ra trước khi chủ động yêu cầu bất kỳ thành viên nào rà soát.
6. Bạn không được ghi công commit cho AI/LLM, kể cả thông qua các trailer như "Assisted-by", "Co-developed-by", hoặc tương tự.
7. Không viết thông điệp commit quá dài. Thông tin quan trọng nên nằm trong mô tả PR thay vì trong các thông điệp commit bị thu gọn.
8. Nếu bạn không sẵn lòng hoặc không thể làm tất cả những điều trên, vui lòng đóng issue hoặc pull request của bạn.

Cảm ơn!

## Đóng góp cho tài liệu

Tài liệu là một phần quan trọng của OpenCodeReview. Chúng tôi hoan nghênh các cải thiện đối với tệp README, chú thích mã nội dòng, ví dụ cấu hình, và mọi văn bản hướng tới người dùng.

### Điều gì được tính là đóng góp tài liệu

- Sửa lỗi chính tả, lỗi ngữ pháp, hoặc liên kết hỏng
- Làm rõ các giải thích gây khó hiểu hoặc bổ sung ngữ cảnh còn thiếu
- Thêm ví dụ sử dụng cho các lệnh hoặc tùy chọn cấu hình
- Cập nhật nội dung lỗi thời (ví dụ: sau khi một tính năng thay đổi)
- Dịch hoặc cải thiện các file README và hướng dẫn đóng góp đã bản địa hóa trong `docs/i18n/`

### Quy trình tài liệu

1. Nếu bạn phát hiện một vấn đề nhưng không định tự sửa, hãy mở một [Documentation Issue](https://github.com/alibaba/open-code-review/issues/new?template=docs_report.yml).
2. Nếu bạn muốn sửa nó, hãy fork kho chứa, thực hiện thay đổi, và gửi một PR với tiền tố nhánh `docs/` (ví dụ: `docs/fix-config-example`).
3. Các PR chỉ về tài liệu không yêu cầu thay đổi kiểm thử, nhưng vui lòng xác minh rằng mọi lệnh hoặc đoạn mã bạn bao gồm đều chính xác.

### Các tệp tài liệu

| File                              | Mục đích                             |
| --------------------------------- | ------------------------------------ |
| `README.md`                       | Tài liệu dự án chính (tiếng Anh)     |
| `docs/i18n/README.zh-CN.md`       | Bản dịch tiếng Trung                 |
| `docs/i18n/README.ja-JP.md`       | Bản dịch tiếng Nhật                  |
| `docs/i18n/README.ko-KR.md`       | Bản dịch tiếng Hàn                   |
| `docs/i18n/README.ru-RU.md`       | Bản dịch tiếng Nga                   |
| `docs/i18n/README.vi-VN.md`       | Bản dịch tiếng Việt                  |
| `CONTRIBUTING.md`                 | Hướng dẫn đóng góp (tiếng Anh)       |
| `docs/i18n/CONTRIBUTING.zh-CN.md` | Hướng dẫn đóng góp (tiếng Trung)     |
| `docs/i18n/CONTRIBUTING.ja-JP.md` | Hướng dẫn đóng góp (tiếng Nhật)      |
| `docs/i18n/CONTRIBUTING.ko-KR.md` | Hướng dẫn đóng góp (tiếng Hàn)       |
| `docs/i18n/CONTRIBUTING.ru-RU.md` | Hướng dẫn đóng góp (tiếng Nga)       |
| `docs/i18n/CONTRIBUTING.vi-VN.md` | Hướng dẫn đóng góp (tiếng Việt)      |

## Gửi thay đổi

### Mở một Issue

Trước khi làm một thay đổi quan trọng, vui lòng mở một issue trước để thảo luận về cách tiếp cận. Điều này ngăn ngừa công việc trùng lặp và đảm bảo đóng góp của bạn phù hợp với định hướng của dự án.

Khi báo cáo lỗi, hãy bao gồm:

1. Phiên bản OpenCodeReview (`ocr version`)
2. Hệ điều hành và kiến trúc
3. Các bước tái hiện
4. Hành vi mong đợi so với hành vi thực tế
5. Nhật ký hoặc thông báo lỗi liên quan

### Quy trình Pull Request

1. **Giữ PR tập trung** — Một thay đổi logic cho mỗi PR. Nếu bạn có nhiều thay đổi độc lập, hãy gửi các PR riêng biệt.
2. **Viết kiểm thử** — Thêm hoặc cập nhật kiểm thử cho mọi thay đổi hành vi.
3. **Cập nhật tài liệu** — Nếu thay đổi của bạn ảnh hưởng tới hành vi hướng tới người dùng, hãy cập nhật tài liệu liên quan.
4. **Ký CLA** — Mọi người đóng góp phải ký Thỏa thuận Giấy phép Người đóng góp trước khi PR của họ có thể được hợp nhất (xem bên dưới).
5. **Điền mẫu PR** — Mô tả thay đổi của bạn làm gì và vì sao.

### Định dạng tiêu đề PR

Sử dụng cùng định dạng Conventional Commits như thông điệp commit:

```
feat(viewer): add session search
```

### Quy trình rà soát

- Một maintainer sẽ rà soát PR của bạn, thường trong vòng vài ngày làm việc.
- Chúng tôi có thể yêu cầu thay đổi — điều này là bình thường và mang tính hợp tác, không phải đối đầu.
- Sau khi được phê duyệt, một maintainer sẽ hợp nhất PR của bạn.

## Mẹo để PR được rà soát nhanh hơn

Muốn PR của bạn được rà soát và hợp nhất nhanh chóng? Những thực hành sau sẽ giúp:

- **Ký CLA sớm** — Nhiều người đóng góp lần đầu bị chặn vì họ bỏ lỡ bình luận của CLA bot. Hãy ký Thỏa thuận Giấy phép Người đóng góp ngay khi bot nhắc bạn — PR của bạn không thể được hợp nhất nếu thiếu nó.
- **Đảm bảo mọi kiểm tra CI đều đạt** — Các PR có kiểm tra thất bại sẽ không được rà soát. Hãy chạy `make test` và `make build` cục bộ trước khi push để phát hiện vấn đề sớm.
- **Giữ thay đổi tập trung và nhỏ** — Một PR làm tốt một việc sẽ dễ rà soát hơn nhiều so với một PR trộn các thay đổi không liên quan. Các PR nhỏ hơn được rà soát nhanh hơn và ít khả năng cần nhiều vòng sửa đổi.
- **Viết mô tả rõ ràng, chính xác** — Giải thích *cái gì* đã thay đổi và *vì sao*. Mô tả phải phản ánh diff thực tế — người rà soát sẽ mất niềm tin khi hai thứ không khớp nhau. Nếu phạm vi đã thay đổi trong quá trình phát triển, hãy cập nhật mô tả trước khi yêu cầu rà soát.
- **Bao gồm kiểm thử cho các thay đổi hành vi** — Tính năng mới hoặc sửa lỗi mà không có kiểm thử sẽ gây ra câu hỏi. Kiểm thử chứng minh tính đúng đắn và giúp người rà soát hiểu hành vi dự kiến.
- **Tuân theo các mẫu mã sẵn có** — Khớp với phong cách, quy ước đặt tên và kiến trúc của mã xung quanh. Sự nhất quán giảm tải nhận thức cho người rà soát và tránh các bình luận rà soát chỉ về phong cách.
- **Phản hồi nhận xét nhanh chóng** — Khi người rà soát yêu cầu thay đổi, hãy xử lý chúng nhanh để giữ chu kỳ rà soát ngắn. Nếu bạn không đồng ý, hãy giải thích lý do của mình thay vì phớt lờ bình luận.

## Thỏa thuận Giấy phép Người đóng góp (CLA)

Chúng tôi yêu cầu mọi người đóng góp ký Thỏa thuận Giấy phép Người đóng góp Mã nguồn mở của Alibaba trước khi chúng tôi có thể hợp nhất các đóng góp của bạn. Điều này đảm bảo rằng dự án có thể được phân phối theo các điều khoản giấy phép của nó.

Khi bạn mở PR đầu tiên của mình, một CLA bot sẽ đăng một bình luận kèm hướng dẫn. Chỉ cần làm theo liên kết để ký điện tử — chỉ mất một phút.

## Người đóng góp lần đầu

Mới tham gia dự án? Hãy tìm các issue có nhãn:

- [`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue) — Các nhiệm vụ nhỏ, phạm vi rõ ràng, lý tưởng để bắt đầu.
- [`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted) — Các issue mà chúng tôi trân trọng sự giúp đỡ của cộng đồng.

Một số lĩnh vực tốt để bắt đầu:

- Cải thiện thông báo lỗi và đầu ra CLI
- Viết kiểm thử cho các đường mã chưa được kiểm thử
- Cải thiện tài liệu

## Cộng đồng

- **Báo cáo lỗi** — [GitHub Issues](https://github.com/alibaba/open-code-review/issues)
- **Đề xuất tính năng** — [GitHub Discussions (Ideas)](https://github.com/alibaba/open-code-review/discussions/categories/ideas) hoặc [Feature Request Issue](https://github.com/alibaba/open-code-review/issues/new?template=feature_request.yml)
- **Câu hỏi & Trợ giúp** — Nếu bạn có bất kỳ câu hỏi nào về việc sử dụng OpenCodeReview, đừng ngần ngại hỏi trong [GitHub Discussions](https://github.com/alibaba/open-code-review/discussions)

## Giấy phép

Bằng việc đóng góp cho OpenCodeReview, bạn đồng ý rằng các đóng góp của bạn sẽ được cấp phép theo [Apache License 2.0](../../LICENSE).

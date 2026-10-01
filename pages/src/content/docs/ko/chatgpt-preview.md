---
title: ChatGPT 로그인 미리보기
---

OpenCodeReview는 OpenAI 공개 Responses API를 통해 자격이 있는 ChatGPT 요금제 사용량을 이용할 수 있습니다. 오픈 소스 CLI의 선택형 미리보기 기능이며 OpenCodeReview가 별도 구독료를 부과하지 않습니다. 계정, 워크스페이스, 지역, 모델 사용 자격은 OpenAI가 결정합니다.

`chatgpt` provider는 공개된 Sign in with ChatGPT 흐름을 사용합니다. `ocr auth login`, `status`, `logout`은 기본적으로 Sign in with ChatGPT를 사용합니다. 명시적인 `--provider chatgpt`도 계속 지원합니다.

## 로그인 및 모델 선택

```bash
ocr auth login --provider chatgpt
ocr auth status --provider chatgpt
ocr llm models
ocr config set provider chatgpt
ocr config set providers.chatgpt.model gpt-6-luna
ocr config set providers.chatgpt.extra_body '{"reasoning":{"effort":"low"}}'
ocr llm test
```

OAuth 등록은 OpenID Connect 및 OpenAI 범위를 모두 요청합니다. `openid`는 신원을 확인하고, `profile` 및 `email`은 계정 정보를 제공하며, `offline_access`는 갱신 토큰을 통한 오프라인 갱신에 사용됩니다. `resource.invoke`는 리소스 호출 권한이고 `chatgpt.tokens.use.direct`는 ChatGPT 요금제 사용량을 직접 사용하기 위한 권한입니다. 최초 등록 요청에는 `agent_name_hint=OpenCodeReview`가 포함됩니다. 커넥터 권한을 요청하지 않으며 Codex CLI 클라이언트 등록을 재사용하지 않습니다. 문서화된 흐름에 필요한 범위만 요청하며 임의 권한은 요청하지 않습니다.

첫 로그인 때 OpenAI가 클라이언트 등록을 발급하며 해당 계정에 보관합니다. 앱의 실제 이름을 최초 등록 힌트로 보내며 설치 간 같은 이름을 사용하고 재로그인 때는 생략합니다. 이 로컬 설치에 연결된 안정적이고 식별 불가능한 `ext_agent_host_id`를 로그인할 때마다 보내며, 같은 설치에는 같은 호스트 ID를 사용합니다.

인증마다 새 state, nonce, PKCE 값을 생성합니다. 코드를 교환하기 전에 callback state를 확인합니다. issuer의 RSA JWKS로 ID 토큰 서명을 검증하고 issuer, audience, subject, 유효 기간, nonce를 확인한 뒤에만 신원과 허용 범위를 신뢰합니다.

모델 목록은 현재 계정에서 볼 수 있는 제안과 표시 이름을 서버 순서로 보여줍니다. 모델을 설정하지 않으면 provider가 목록에서 첫 번째로 표시되는 모델을 선택합니다. 명시적으로 설정한 모델 또는 `--model` slug는 목록에 없어도 그대로 OpenAI에 전송됩니다. 계정에서 모델을 사용할 수 있는지는 OpenAI가 결정합니다. 이 provider에는 컴파일된 모델 제한이나 별칭이 없습니다.

`extra_body`의 reasoning effort는 API 매개변수입니다. 별도의 review `--effort` 플래그는 OpenCodeReview 검토 라운드와 도구 예산을 제어합니다.

로그인하면 시스템 브라우저가 열리고 `127.0.0.1`에서만 대기합니다. `--no-browser`는 한 번 사용하는 로컬 URL을 출력합니다. CLI는 저장된 ID 토큰 힌트가 포함된 인증 URL을 출력하지 않습니다. 이 미리보기는 장치 코드 인증을 지원하지 않습니다.

로그인에 성공했지만 ChatGPT 요금제 권한이 없으면 OpenCodeReview는 계정 신원을 보존하고 요금제 사용이 비활성화되었다고 알립니다. 현재 저장된 등록에서 동의 화면을 요청하려면 다음을 실행합니다.

```bash
ocr auth login --provider chatgpt --enable-plan
# 다른 저장 등록의 경우:
ocr auth login --provider chatgpt --enable-plan --account <saved-client-id>
```

이 명시적 작업은 `prompt=consent`, 전체 요청 범위와 리소스, 저장된 클라이언트 및 호스트 ID를 전송합니다. `--new-account`와 함께 사용할 수 없습니다. 일반 재로그인은 동의를 강제하지 않습니다. 반환된 grant가 요금제 사용을 허용할 때까지 추론은 비활성 상태입니다. 대신 API 키 provider를 설정할 수 있으며 OpenCodeReview가 결제 경로를 자동 전환하지 않습니다.

## 계정 및 로그아웃

```bash
ocr auth login --provider chatgpt --new-account
ocr auth status --provider chatgpt
ocr auth select <saved-client-id> --provider chatgpt
ocr auth login --provider chatgpt --account <saved-client-id>
ocr auth logout --provider chatgpt --account <saved-client-id>
```

발급된 각 클라이언트 ID는 검증된 issuer와 subject에 연결됩니다. 여러 등록이 같은 이메일을 사용해도 서로 구분됩니다. 다시 로그인하면 같은 클라이언트 ID를 재사용합니다. 코드 교환에서 `invalid_grant`가 발생하면 콜백에서 발급된 클라이언트 ID와 새 state, nonce, PKCE로 인증을 한 번 다시 시작합니다. 서명된 신원을 검증하기 전에 대기 중 등록을 활성화하지 않습니다. 두 번째도 실패하면 기존 자격 증명을 보존하고 로그인을 중단합니다.

자격 증명 데이터베이스는 `~/.opencodereview/auth/chatgpt/accounts.json`입니다. 별도의 `host.json`은 안정적인 설치 호스트 ID를 저장합니다. 자격 증명 파일은 원자적으로 교체됩니다.

Unix에서는 자격 증명 파일에 소유자만 읽고 쓸 수 있는 `0600`, 저장소 디렉터리에 `0700` 권한을 설정합니다. Windows의 접근 권한은 로컬 사용자 프로필에서 상속된 ACL에 따라 결정되며, OpenCodeReview는 DACL을 설정하거나 검증하지 않습니다. Go의 [`os.Chmod`](https://pkg.go.dev/os#Chmod)는 Windows에서 읽기 전용 속성만 변경합니다. ACL로 본인 계정에만 접근을 제한한 사용자 프로필을 사용하고 로그인 전에 권한을 확인하세요. 공유되거나 광범위한 접근이 허용되는 저장 위치는 피하세요. 사용자 프로필 안에 있는 디렉터리라는 이유만으로 비공개 접근이 보장되지는 않습니다.

저장소 변경 및 갱신 토큰 회전에는 OS 파일 잠금을 사용하지만 브라우저 인증은 잠금 바깥에서 진행되어 다른 계정을 계속 사용할 수 있습니다. 완료 시 저장소를 다시 읽고 등록 revision을 확인합니다. 해당 등록의 로그아웃, 갱신 또는 다른 로그인 완료는 대기 중 결과를 무효화합니다. 다른 계정의 업데이트는 보존됩니다. 만료 전에 회전형 갱신 토큰으로 액세스 토큰을 갱신합니다.

로그아웃은 갱신 가능한 세션의 폐기를 시도하고 액세스, 갱신 및 ID 토큰을 지우며 계정 등록과 호스트 ID는 보존합니다. 로그아웃 후 로그인은 `id_token_hint`를 보내지 않습니다. 원격 폐기를 확인할 수 없으면 CLI가 로컬 토큰을 지웠음을 알리고 ChatGPT 설정에서 앱 연결을 해제하도록 안내합니다.

## 전송 및 사용량

이 provider는 `https://api.openai.com/v1/responses`로만 요청을 보냅니다. 엔드포인트, 프로토콜 및 자격 증명 재정의는 거부됩니다. `store: false`로 스트리밍하고 전체 기록을 전송하며 로컬 함수를 `ocr` 네임스페이스로 묶고 후속 턴에 필요한 호출 ID와 암호화된 reasoning을 보존합니다. `response.completed`만 성공으로 처리합니다. 출력이 시작된 뒤 발생한 사용량 제한 오류를 포함해 실패, 미완료, 중단된 스트림은 요청을 중지합니다.

미리보기 요청 필드는 OpenAI 지원 하위 집합으로 제한됩니다. `extra_body`는 `reasoning`, `text`, `prompt_cache_key`를 받으며 모델, 기록, 도구, 스트림 모드, 저장 정책을 바꿀 수 없습니다. 응답에서 provider 원본 사용량을 확인할 수 있고, 기존 선택형 원시 검토 기록은 자격 증명 헤더 없이 스트림을 기록합니다.

**ChatGPT 요금제 사용:** 자격이 있는 요청은 선택한 계정의 ChatGPT 요금제를 사용합니다. [ChatGPT 설정에서 사용량 관리](https://chatgpt.com/settings/usage). 사용량 제한 오류는 요금제 전체 또는 앱별 한도를 반영할 수 있습니다. CLI는 재설정 시각을 추측하지 않습니다.

현재 요구 사항은 OpenAI의 [오픈 소스 SIWC 개요](https://developers.openai.com/siwc/token-sharing-open-source), [로그인 계약](https://developers.openai.com/siwc/token-sharing-open-source/sign-in), [미리보기 제한](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)을 참조하세요.

`ocr auth` 및 `ocr llm models` 명령은 [CLI 레퍼런스](../cli-reference/)를, provider 설정은 [구성 안내](../configuration/)를 참조하세요.

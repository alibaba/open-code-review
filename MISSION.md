# Mission: 讀懂 OpenCodeReview 的 review runtime

## Why

我想在 open-code-review 裡追完一次 review 從 CLI 入口、diff、prompt、LLM、tools 到輸出與 session 的實際路徑，並能用 harness 的語言對照它，之後可以自己 debug 或修改 review pipeline。

## Success looks like

- 能從 executeReviewContextWithStage 追到每檔案的 RunPerFile。
- 能說清楚 prompt、ToolDef、tool Provider、LLM client 各自在哪一層。
- 能用一個 finding 說明 code_comment 如何從模型輸出變成最終 review 結果。
- 能分辨 diff review 與 full scan 共用哪些元件，以及在哪裡分流。

## Constraints

- 以 repository source 為主要教材，先讀 diff review，再延伸到 full scan。
- 每課只處理一個可驗證的心智模型，使用短的 retrieval practice。
- 說明使用台灣正體中文，保留程式碼中的原名。

## Out of scope

- 先不深入 provider endpoint 的所有設定分支。
- 先不把 VS Code extension 與網站 UI 當成 review core。

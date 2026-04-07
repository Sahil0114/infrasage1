# LLM.md — Model and Ollama Instructions

> Read this before writing anything in `internal/llm/`.

---

## Model Selection Rationale

**Chosen model:** `qwen2.5-coder:3b` (base, via Ollama)

Why this model over alternatives:

| Model | RAM (Q4) | HCL Quality | Why Not Used |
|---|---|---|---|
| `qwen2.5-coder:3b` ✅ | ~2.0 GB | Good | Our choice |
| `deepseek-coder:6.7b` | ~4.5 GB | Better | Too much RAM with containers |
| `codellama:7b` | ~4.8 GB | Similar | Too much RAM with containers |
| `phi3:mini` | ~2.3 GB | Mediocre | Poor HCL understanding |

On M2 with 8GB, `qwen2.5-coder:3b` is the only model that fits comfortably alongside Docker Desktop and the five service containers. The model is used as-is from Ollama — no fine-tuning is applied. Output quality is achieved through careful system prompt engineering (see `ARCHITECTURE.md`).

---

## How Ollama Works (For Reference)

Ollama is a local model server. It:
1. Manages model files stored in `~/.ollama/models/` (not in the project)
2. Exposes a REST API on `http://localhost:11434`
3. Uses Apple Metal GPU on M2 for inference (fast)
4. Loads model into memory on first request, keeps it loaded

**Key Ollama commands (human runs these, not the Go CLI):**
```bash
ollama serve                    # Start server (if not running as system service)
ollama pull qwen2.5-coder:3b   # Download model (~2GB, one-time)
ollama list                     # Show downloaded models + RAM usage
ollama run qwen2.5-coder:3b    # Interactive chat (for testing)
ollama ps                       # Show currently loaded models
ollama stop qwen2.5-coder:3b   # Unload from memory
```

**The `ollama serve` process:** On macOS, `brew install ollama` installs Ollama as both a CLI and an optional background service. Either `ollama serve` in a terminal or `brew services start ollama` works. The Go CLI's `stack up` command checks if it's running and starts it if not.

---

## Ollama REST API — The Only Endpoint We Use

**Endpoint:** `POST http://localhost:11434/api/generate`

**Request body:**
```json
{
  "model": "qwen2.5-coder:3b",
  "prompt": "create an S3 bucket with versioning",
  "system": "You are an expert Terraform engineer...",
  "stream": false,
  "options": {
    "temperature": 0.1,
    "num_ctx": 4096,
    "top_p": 0.9
  }
}
```

**Response body:**
```json
{
  "model": "qwen2.5-coder:3b",
  "response": "terraform {\n  required_providers {\n    aws = {\n      source  = \"hashicorp/aws\"\n...",
  "done": true,
  "total_duration": 12345678900,
  "eval_count": 312
}
```

**The field we use:** `response` — this is the raw model output.

**Health check endpoint:** `GET http://localhost:11434/api/tags`
Returns list of available models. Status 200 = server is up.

---

## Temperature Setting — Why 0.1

Temperature controls randomness. At 0.1 (very low):
- The model makes deterministic, confident choices
- HCL output is consistent across multiple runs of the same prompt
- The model won't "get creative" with invalid Terraform syntax

At 0.7+ (higher temperature):
- Output varies between runs
- Risk of hallucinated resource types or invalid attribute names

For code generation, always use low temperature. 0.1 is the correct value.

---

## Context Window — Why 4096

`num_ctx: 4096` sets the context window (in tokens). This means:
- System prompt (~200 tokens) + user prompt (~50 tokens) + response (up to ~3800 tokens)
- Most Terraform files are 50–300 lines, fitting comfortably within this

Do not increase above 4096 for this model — it significantly increases RAM usage and slows inference.

---

## The System Prompt — Detailed Explanation

The system prompt is a constant defined in `cmd/ask.go`. Here is why each rule exists:

```
Rule 1: "Output ONLY raw HCL code"
→ Without this, models add explanations like "Here is your Terraform code:"
→ These explanations break the HCL parser

Rule 2: "Start your response with the first line of HCL code"
→ Reinforces rule 1. Some models still add a leading sentence despite rule 1.

Rule 3 & 4: "Always include terraform { required_providers } block"
→ Checkov fails without this block
→ Terraform itself requires it for newer versions

Rule 6 (security defaults):
→ Makes the base model generate secure-by-default HCL
→ Without this, models generate the simplest possible HCL which often fails Checkov

Rule 7 (tags):
→ Tags are best practice for AWS cost allocation and IaC tracking
→ Checkov checks for tags — having them reduces finding count
```

---

## HCL Extraction — Why It's Needed

After calling the LLM, the raw response may look like any of these:

**Case 1 — Fenced with language hint (most common):**
````
```hcl
terraform {
  ...
}
```
````

**Case 2 — Fenced without language hint:**
````
```
terraform {
  ...
}
```
````

**Case 3 — No fencing (ideal but rare with base model):**
```
terraform {
  ...
}
```

**Case 4 — Explanation then code (bad model behaviour):**
```
Here is your Terraform HCL for an S3 bucket:

terraform {
  ...
}
```

**Case 5 — Explanation and fencing:**
````
Sure! Here is the code:
```hcl
terraform {
  ...
}
```
````

The `ExtractHCL` function in `internal/terraform/parser.go` handles all five cases. The regex for fences handles cases 1, 2, and 5. The HCL content check (`strings.Contains(lower, "terraform {")`) handles case 4 by returning everything from the start of the HCL keyword. Case 3 passes through directly.

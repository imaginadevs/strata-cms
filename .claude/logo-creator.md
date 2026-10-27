---
name: logo-generator
description: Analyze a repository's README and business/project name to design project logo concepts, generating ready-to-use scalable SVG code alongside text prompts for AI image generators. Trigger when the user asks to create, design, or generate a logo for a project, repo, or business name.
---

# Logo Generator

Analyze repository documentation and brand identity inputs to create tailored project logo concepts. This skill extracts core project attributes, generates scalable SVG code directly renderable in browsers, and provides optimized text prompts for AI image generators.

## When to Use

Use this skill when the user asks to:
- Generate, design, or craft a logo for a project, repository, or business name.
- Analyze a `README.md` or repository file to propose branding or visual identity elements.
- Create SVG logos or AI image generation prompts based on codebase context.

## Workflow Steps

### 1. Context Extraction
- Locate and read the project's primary documentation (e.g., `README.md`, `package.json`, or root overview).
- Identify key attributes:
  - **Core Functionality:** What problem does the project solve?
  - **Target Audience & Tone:** Is it developer-focused, enterprise, playful, or minimalist?
  - **Metaphors & Keywords:** Extract 3-5 visual metaphors or symbols associated with the project's domain (e.g., fast -> lightning, secure -> shield, data pipeline -> flowing nodes).

### 2. Design Concept Formulation
Develop 2 distinct visual concepts based on the extracted context:
1. **Modern Minimalist / Technical Mark:** Icon-driven, clean geometric lines, suitable for developer tools or CLI applications.
2. **Abstract / Domain Metaphor:** Creative combination of the brand name initial and domain function.

For each concept, specify:
- **Palette:** 2-3 dominant hex color codes matching the project's tone.
- **Typography Suggestion:** Clean sans-serif, monospaced, or serif alignment.
- **Visual Rationale:** Explanation linking design choices directly to the project's features.

### 3. Deliverable Generation

For each proposed concept, deliver both an SVG implementation and an AI prompt:

#### A. Scalable Vector Graphics (SVG)
- Output production-ready, clean, standalone SVG code block (`xml` or `svg` fence).
- Ensure high visibility on both light and dark backgrounds (use `viewBox`, clean paths, vector geometries).
- Embed styling within SVG or use inline attributes for self-contained rendering.

#### B. AI Image Generator Prompt
- Draft a detailed, structured prompt for midjourney/DALL-E 3/Flux.
- Follow the prompt structure: `[Subject/Logo type], [Core symbols], [Style/Medium], [Color scheme], [Background/Lighting constraints] --no realistic photos, text, drop shadows`.

## Guidelines & Constraints
- **Format Default:** SVG is the primary format for web applications and repos (vector, scalable, responsive).
- **No Dependencies:** SVG code must not rely on external font files or remote assets. Use path shapes or standard vector geometries for text and icons.
- **Keep Code Clean:** Maintain clean indentation and valid vector markup.
# RiseFit Knowledge Graph: 4-Dimensional Metadata Specification

This document defines the metadata tagging system used to categorize and prioritize scientific papers, books, and resources within the RiseFit Knowledge Graph. Every document ingested into the system is automatically tagged across these four dimensions using Gemini Multimodal Analysis.

---

## Dimension 1: Core Topic (`category`)
Defines the primary scientific domain of the content.

| Tag | Description (CN) | Use Case |
|:--- |:--- |:--- |
| `hypertrophy` | 增肌机制 | Muscle protein synthesis, volume, intensity, and hypertrophy research. |
| `fat_loss` | 减脂与能量消耗 | Caloric deficit, metabolic rate, thermogenesis, and lipid metabolism. |
| `nutrition` | 营养与补剂 | Macronutrients, micronutrients, creatine, protein timing, etc. |
| `biomechanics` | 生物力学与动作 | Lever arms, torque, exercise technique, and structural anatomy. |
| `recovery` | 恢复与睡眠 | Deloading, CNS fatigue, sleep hygiene, and active recovery. |

---

## Dimension 2: Trigger Intent (`intent`)
Determines the functional scenario where the AI should prioritize this document.

| Tag | Description (CN) | AI Behavior |
|:--- |:--- |:--- |
| `programming` | 训练计划生成 | Used when generating or adjusting periodization and workout routines. |
| `myth_busting` | 伪科学反驳 | High priority when a user asks about "spot reduction" or "detox diets." |
| `form_correction` | 动作姿态纠正 | Triggered when analyzing user videos or describing exercise execution. |
| `general_guidance` | 日常知识问答 | Default source for general fitness education and "How-to" questions. |

---

## Dimension 3: Target Audience (`target_audience`)
Ensures the advice is personalized and safe for specific demographics.

| Tag | Description (CN) | Context |
|:--- |:--- |:--- |
| `general` | 所有人 | Standard biological principles applicable to most healthy adults. |
| `female` | 女性专属 | Focuses on menstrual cycle phase training, pregnancy, or female physiology. |
| `special_population` | 特殊人群 | Protocols for elderly, hypertensive, diabetic, or injured individuals. |

---

## Dimension 4: Evidence Level (`evidence_level`)
Determines the "authority" and "hardness" of the AI's response.

| Tag | Description (CN) | Priority | AI Tone |
|:--- |:--- |:--- |:--- |
| `position_stand` | 官方立场声明 | **Highest** | "The scientific consensus states..." (Non-negotiable) |
| `meta_analysis` | 元分析综述 | **High** | "Evidence strongly suggests..." (Strongly recommended) |
| `textbook` | 教材基础理论 | **Base** | "Fundamentally, the mechanism is..." (Educational/Fallback) |

---

## Technical Implementation

### Extraction
The system uses the **Gemini 2.5 Flash** multimodal engine to analyze the **first page** (Title, Abstract, Introduction) of every PDF. The extraction logic is contained in `pkg/pdf/processor.go` under the `smartCategorize` method.

### Storage
Tags are stored in two locations within Qdrant:
1.  **`risefit_content` (Page-Level)**: Every searchable page carries these tags in its payload for real-time filtering during RAG.
2.  **`risefit_registry` (Document-Level)**: A single record per file for library management and deduplication.

### Search Filtering (Example)
When a user asks: *"How should I train during my period?"*
The AI should construct a Qdrant query with a filter:
```json
{
  "must": [
    { "key": "target_audience", "match": { "value": "female" } },
    { "key": "category", "match": { "value": "recovery" } }
  ]
}
```

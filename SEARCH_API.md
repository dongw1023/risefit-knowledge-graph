# Risefit Search API Documentation

The Search API provides a high-level interface for performing semantic similarity searches against the Risefit Knowledge Graph. It leverages vector embeddings to find relevant document chunks based on natural language queries.

## Base URL
The service runs on port `8000` by default.
`http://<host>:8000`

## Endpoints

### 1. Search Documents
Performs a semantic search using a text query and optional metadata filters.

*   **URL:** `/v1/search`
*   **Method:** `POST`
*   **Content-Type:** `application/json`

#### Request Body
| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `query` | `string` | Yes | The natural language search query. |
| `num_results` | `integer` | No | Number of results to return (Default: 5). |
| `filters` | `object` | No | Metadata filters to narrow down results. |

**Filters Object:**
| Field | Type | Description |
| :--- | :--- | :--- |
| `category` | `string` | Filter by document category (e.g., "Nutrition", "Exercise"). |
| `intent` | `string` | Filter by user intent (e.g., "Informational", "Instructional"). |
| `target_audience` | `string` | Filter by audience (e.g., "Beginner", "Advanced"). |
| `evidence_level` | `string` | Filter by scientific evidence level. |

**Example Request (Comprehensive):**
```json
{
  "query": "How to perform a proper barbell squat for strength gains?",
  "num_results": 10,
  "filters": {
    "category": "Exercise Technique",
    "intent": "Instructional",
    "target_audience": "Intermediate",
    "evidence_level": "Expert Consensus"
  }
}
```

#### Response Body
Returns a list of search results sorted by relevance score.

| Field | Type | Description |
| :--- | :--- | :--- |
| `results` | `array` | List of result objects. |
| `results[].content` | `string` | The text content of the document chunk. |
| `results[].score` | `float` | Similarity score (higher is more relevant). |
| `results[].metadata` | `object` | Associated metadata (source file, page number, categories, etc.). |

**Example Response (Comprehensive):**
```json
{
  "results": [
    {
      "content": "A proper barbell squat starts with feet shoulder-width apart. Maintain a neutral spine and descend until thighs are parallel to the floor...",
      "score": 0.985,
      "metadata": {
        "source": "strength_training_fundamentals.pdf",
        "page": 45,
        "category": "Exercise Technique",
        "intent": "Instructional",
        "target_audience": "Intermediate",
        "evidence_level": "Expert Consensus",
        "author": "Dr. Smith",
        "tags": ["legs", "compound", "strength"]
      }
    }
  ]
}
```

## Error Codes
| Status Code | Description |
| :--- | :--- |
| `200` | Success. |
| `400` | Bad Request (e.g., missing query, invalid JSON). |
| `405` | Method Not Allowed (must use POST). |
| `500` | Internal Server Error (e.g., vector store connection failure). |

## Example Usage (cURL)
```bash
curl -X POST http://localhost:8000/v1/search \
     -H "Content-Type: application/json" \
     -d '{
           "query": "muscle recovery protein",
           "num_results": 1
         }'
```

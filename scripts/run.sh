    curl -X POST http://localhost:8080/search \
         -H "Content-Type: application/json" \
         -d '{
           "query": "What is the content of the cover confirmation?",
           "num_results": 3
         }'
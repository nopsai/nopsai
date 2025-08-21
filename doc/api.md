docker-compose down -v && docker container prune -f && docker volume prune -f && docker image prune -f

# Secrets

// General
curl http://localhost:8080/v1/secrets

curl -X DELETE http://localhost:8080/v1/secrets/TEST_SECRET

curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value": "General level secret"}' \
  http://localhost:8080/v1/secrets/TEST_SECRET


// Repositories
curl http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets

curl -X DELETE http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets/TEST_SECRET

curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value": "repo level secret"}' \
  http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets/TEST_SECRET




# Pipelines  

curl http://localhost:8080/v1/pipelines

curl http://localhost:8080/v1/pipelines/main-pipeline.yaml

curl -X DELETE http://localhost:8080/v1/pipelines/main-pipeline.yaml

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/main-pipeline.yaml" \
  http://localhost:8080/v1/pipelines/main-pipeline.yaml



# Trigger

curl http://localhost:8080/v1/overrides

curl http://localhost:8080/v1/overrides/hosein-yousefii/test-app

curl -X DELETE http://localhost:8080/v1/overrides/hosein-yousefii/test-app

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/triggers.yaml" \
  http://localhost:8080/v1/overrides/hosein-yousefii/test-app

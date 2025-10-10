docker-compose down -v ; docker container prune -f && docker volume prune -f && docker image prune -f

curl -X PUT -d '{"value": "tcp://192.168.1.16:2375"}' \
  'http://localhost:8080/v1/environments/DOCKER_HOST?env=prod'

curl -X PUT -d '{"value": "general prod"}' \
  'http://localhost:8080/v1/environments/TEST_ENV?env=prod'

curl -X PUT -d '{"value": "general prod"}' \
  'http://localhost:8080/v1/environments/API_VERSION?env=prod'

curl -X PUT -d '{"value": "tcp://192.168.1.16:2375"}' \
  'http://localhost:8080/v1/environments/DOCKER_HOST'

curl -X PUT -d '{"value": "general prod"}' \
  'http://localhost:8080/v1/environments/API_VERSION'

curl -X PUT -d '{"value": "General level secret prod env"}' \
  'http://localhost:8080/v1/secrets/TEST_SECRET?env=prod'

curl -X PUT -d '{"value": "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"}' \
  'http://localhost:8080/v1/secrets/GITHUB_PIPELINE_TOKEN?env=prod'


# Secrets

## General
curl http://localhost:8080/v1/secrets?env=prod

curl -X DELETE http://localhost:8080/v1/secrets/TEST_SECRET

curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value": "General level secret"}' \
  http://localhost:8080/v1/secrets/TEST_SECRET


## General with env

curl -X PUT -d '{"value": "General level secret prod env"}' \
  'http://localhost:8080/v1/secrets/TEST_SECRET?env=prod'



## Repository
curl http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets

curl -X DELETE http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets/TEST_SECRET

curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"value": "repo level secret"}' \
  http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets/TEST_SECRET

## Repository with env

curl -X PUT -d '{"value": "repo level secret prod env"}' \
  'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/secrets/TEST_SECRET?env=prod'



_______________________________________-

# environments

## General
curl -X PUT -d '{"value": "general"}' \
  'http://localhost:8080/v1/environments/TEST_ENV'

curl 'http://localhost:8080/v1/environments/'

curl -X 'DELETE http://localhost:8080/v1/environments/TEST_ENV'

## General with env
curl -X PUT -d '{"value": "general prod"}' \
  'http://localhost:8080/v1/environments/TEST_ENV?env=prod'

curl 'http://localhost:8080/v1/environments?env=prod'

curl -X DELETE 'http://localhost:8080/v1/environments/TEST_ENV?env=prod'


## Repository
curl -X PUT -d '{"value": "repo"}' \
  'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/environments/TEST_ENV'

curl 'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/environments'

curl -X DELETE 'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/environments/TEST_ENV'


## Repository with env
curl -X PUT -d '{"value": "repo prod"}' \
  'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/environments/TEST_ENV?env=prod'

curl 'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/environments?env=prod'

curl -X DELETE 'http://localhost:8080/v1/repositories/hosein-yousefii/test-app/environments/TEST_ENV?env=prod'

_______________________________________-

# Pipelines

curl http://localhost:8080/v1/pipelines

curl http://localhost:8080/v1/pipelines/main-pipeline

curl http://localhost:8080/v1/pipelines/team-1/dev/main-pipeline

curl -X DELETE http://localhost:8080/v1/pipelines/team-1/dev/main-pipeline

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/main-pipeline.yaml" \
  http://localhost:8080/v1/pipelines/main-pipeline

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/team-1/dev/main-pipeline.yaml" \
  http://localhost:8080/v1/pipelines/team-1/dev/main-pipeline



# Trigger

curl http://localhost:8080/v1/overrides

curl http://localhost:8080/v1/overrides/hosein-yousefii/test-app

curl -X DELETE http://localhost:8080/v1/overrides/hosein-yousefii/test-app

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/triggers.yaml" \
  http://localhost:8080/v1/overrides/hosein-yousefii/test-app

_______________________________________-

# Steps

curl http://localhost:8080/v1/steps

curl http://localhost:8080/v1/steps/simple-step

curl http://localhost:8080/v1/steps/shared/utilities/archive-step

curl -X DELETE http://localhost:8080/v1/steps/shared/utilities/archive-step

curl -X PUT \
  -H "Content-Type: application/x-yaml" \
  --data-binary "@.nopsai/shared/utilities/archive-step.yaml" \
  http://localhost:8080/v1/steps/shared/utilities/archive-step

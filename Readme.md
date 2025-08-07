

docker-compose up --build -d


curl -X POST -H "Content-Type: application/x-yaml" --data-binary "@sample-pipeline/2-pipeline.yaml" http://localhost:8080/v1/run
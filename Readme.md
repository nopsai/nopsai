
docker build . -t nopsai
docker-compose up --build -d


curl -X POST -H "Content-Type: application/x-yaml" --data-binary "@sample-pipeline/1-pipeline.yaml" http://localhost:8080/v1/run
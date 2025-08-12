```
SECRET="vsfdverguhuyi3287467324ujfbsaihufb"
PAYLOAD=$(cat doc/sample-git-event.json)
SIGNATURE=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | sed 's/^.* //')
echo "sha256=$SIGNATURE"
```

# Make sure you've already run the openssl command above to set the SIGNATURE variable

curl -X POST \
  -H "Content-Type: application/json" \
  -H "X-GitHub-Event: push" \
  -H "X-Hub-Signature-256: sha256=$SIGNATURE" \
  --data-binary "@doc/sample-git-event.json" \
  http://localhost:8081/webhook
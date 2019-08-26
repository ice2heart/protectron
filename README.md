## Build

```
docker build -t ice2heart/protectron:latest .
docker push ice2heart/protectron:latest
```

## Install

```
docker run -d --restart unless-stopped -e API_TOKEN='TELEGRAM_BOT_TOKEN' --name protectron ice2heart/protectron
```
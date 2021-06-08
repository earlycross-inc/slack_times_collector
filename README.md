# Slack Times Collector
A Slack app collects "times-" channels' updates.

## Structure
- GCP Cloud Function `collect-times`: collects times update and post result once an hour.
    + triggered by periodic event from Cloud Scheduler (with Pub/Sub)
    
- GCP Cloud Function `handle-app-home-open`: renders an App Home tab UI when an user opens the App Home.
- GCP Cloud Function `toggle-watch-state`: toggles a channel watching state when an user clicks a toggle button on the App Home tab.
    + triggered by HTTP request from Slack
    
- API Gateway `slack-times-collector`: endpoint for events (as HTTP request) from Slack.

```
/
├─ collect.go: codes for "collect-times"
├─ app_open.go: codes for "handle-app-home-open"
├─ toggle_watch_state.go: codes for "toggle-watch-state"
├─ common.go: common codes for GCP Cloud Functions
└─ openapi2-functions.yaml: API definition (mappings from URL to Cloud Function) for the API Gateway
```

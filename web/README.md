# Pyxis Web frontend

The responsive web dashboard is served from `internal/webapp/static/` so the Go binary can embed it directly.

`app.js` is implemented as a browser-native React module that imports React and ReactDOM from `esm.sh`, which keeps the repository lightweight while still delivering a React UI for desktop and mobile browsers.

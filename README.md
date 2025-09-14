# dns-zone-receiver

## 未実装

- リクエストサイズの制限
  - 前段に Envoy を挟んで [max_request_bytes](https://www.envoyproxy.io/docs/envoy/latest/api-v3/extensions/filters/http/buffer/v3/buffer.proto#extensions-filters-http-buffer-v3-buffer) を挟めばよい
- アクセス制限
  - 前段に Envoy を挟んで [External Authorization](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_authz_filter) を挟めばよい

## dev memo

```shell
git tag $(date --utc +%Y%m%d-%H%M%S)-$(git rev-parse --short HEAD)
```

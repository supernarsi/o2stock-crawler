# 接口签名规范文档

为了保证 API 调用的安全性，o2stock-api 引入了基于 HMAC-SHA256 的接口签名机制。所有客户端请求（除特定公开接口外）均需进行签名验证。

## 1. 签名生成规则

### 1.1 算法
HMAC-SHA256

### 1.2 签名密钥
使用项目配置的 `SIGNATURE_SECRET`。

### 1.3 签名内容
签名原字符串（Raw String）由以下部分拼接而成（无分隔符）：
`Method` + `RequestURI` + `Timestamp` + `Nonce` + `BodyDigest`

- **Method**: 请求方法，全大写（如 `GET`, `POST`）。
- **RequestURI**: 请求路径，包含查询参数（如 `/players?page=1`）。
- **Timestamp**: Unix 时间戳（秒级），字符串格式。
- **Nonce**: 随机字符串，至少 16 位。
- **BodyDigest**: 请求体的 SHA256 哈希值（十六进制小写）。如果请求体为空，则对空字节数组进行哈希。

### 1.4 生成步骤
1. 计算 `BodyDigest` = Hex(SHA256(RequestBody))
2. 拼接 `Raw` = Method + RequestURI + Timestamp + Nonce + BodyDigest
3. 计算 `Signature` = Hex(HMAC-SHA256(Secret, Raw))

## 2. 请求头要求

请求必须包含以下 Header：

| Header 字段 | 说明 | 示例 |
|---|---|---|
| `x-signature` | 生成的签名值（十六进制小写） | `a1b2c3d4...` |
| `x-timestamp` | 当前 Unix 时间戳（秒） | `1678888888` |
| `x-nonce` | 随机字符串（至少16位） | `randomstring1234` |

## 3. 安全策略

- **时间戳校验**: 服务器只接受当前时间 ±5 分钟内的请求。
- **防重放**: `x-nonce` 在短期内（10分钟）不可重复使用。
- **完整性**: 签名包含请求体摘要，防止篡改。

## 4. 调试模式

在开发或调试阶段，可通过设置 Header 跳过签名校验（**生产环境请慎用**）：

- `xdebug`: `42`

## 5. 错误码

- `401 Unauthorized`:
  - `Missing signature headers`: 缺少签名头。
  - `Invalid timestamp`: 时间戳格式错误。
  - `Timestamp expired`: 时间戳过期（超过 ±5 分钟）。
  - `Nonce too short`: 随机字符串长度不足 16 位。
  - `Nonce reused`: 随机字符串已被使用（重放攻击）。
  - `Invalid signature`: 签名校验失败。

## 6. 代码示例

### Go 示例

```go
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"
)

func main() {
	secret := "your_secret_key"
	method := "POST"
	uri := "/player/in"
	body := []byte(`{"player_id": 10001}`)
	nonce := "random_nonce_123456"
	ts := fmt.Sprintf("%d", time.Now().Unix())

	// 1. Calculate Body Digest
	hash := sha256.Sum256(body)
	bodyDigest := hex.EncodeToString(hash[:])

	// 2. Construct Raw String
	raw := fmt.Sprintf("%s%s%s%s%s", method, uri, ts, nonce, bodyDigest)

	// 3. Calculate Signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(raw))
	signature := hex.EncodeToString(mac.Sum(nil))

	// 4. Send Request
	req, _ := http.NewRequest(method, "http://localhost:8080"+uri, bytes.NewBuffer(body))
	req.Header.Set("x-signature", signature)
	req.Header.Set("x-timestamp", ts)
	req.Header.Set("x-nonce", nonce)
    req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, _ := client.Do(req)
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	fmt.Println(string(respBody))
}
```

### Python 示例

```python
import hashlib
import hmac
import time
import requests

secret = b"your_secret_key"
method = "POST"
uri = "/player/in"
body = b'{"player_id": 10001}'
nonce = "random_nonce_123456"
ts = str(int(time.time()))

# 1. Body Digest
body_digest = hashlib.sha256(body).hexdigest()

# 2. Raw String
raw = f"{method}{uri}{ts}{nonce}{body_digest}".encode()

# 3. Signature
signature = hmac.new(secret, raw, hashlib.sha256).hexdigest()

# 4. Request
headers = {
    "x-signature": signature,
    "x-timestamp": ts,
    "x-nonce": nonce,
    "Content-Type": "application/json"
}
resp = requests.post("http://localhost:8080" + uri, data=body, headers=headers)
print(resp.text)
```

### JavaScript (Node.js) 示例

```javascript
const crypto = require('crypto');
const axios = require('axios');

const secret = 'your_secret_key';
const method = 'POST';
const uri = '/player/in';
const body = JSON.stringify({ player_id: 10001 });
const nonce = 'random_nonce_123456';
const ts = Math.floor(Date.now() / 1000).toString();

// 1. Body Digest
const bodyDigest = crypto.createHash('sha256').update(body).digest('hex');

// 2. Raw String
const raw = `${method}${uri}${ts}${nonce}${bodyDigest}`;

// 3. Signature
const signature = crypto.createHmac('sha256', secret).update(raw).digest('hex');

// 4. Request
axios.post('http://localhost:8080' + uri, JSON.parse(body), {
    headers: {
        'x-signature': signature,
        'x-timestamp': ts,
        'x-nonce': nonce
    }
}).then(res => {
    console.log(res.data);
}).catch(err => {
    console.error(err.response ? err.response.data : err.message);
});
```

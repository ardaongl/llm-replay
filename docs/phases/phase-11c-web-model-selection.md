# Phase 11C — Web Model Selection & One-Command Arena

## Amaç

Phase 11 Arena'yı terminalde model adı yazmadan başlatmak ve iki modeli canlı
web arayüzünden seçmek. Ana kullanıcı yolu:

```bash
make arena
```

Bu alt faz, Phase 11'in sabit iki CLI modeli sözleşmesini değiştirir; güvenlik
sınırlarını ve dataset kayıt modelini değiştirmez.

## Ürün Sözleşmesi

- `llm-replay arena` model argümanı istemez.
- Arayüz Model A ve Model B için iki dropdown gösterir.
- Dropdown yalnızca binary içine gömülü, server-side allowlist'i gösterir.
- Serbest model adı girilemez ve provider katalogları uzaktan keşfedilmez.
- Eksik API anahtarına ait modeller görünür ama seçilemez.
- Aynı model iki tarafta seçilemez.
- Tek provider anahtarıyla o provider'a ait iki farklı model karşılaştırılabilir.
- API anahtarları, base URL'ler ve dataset path tarayıcıya gönderilmez.

Başlangıç kataloğu, mevcut text-only provider adaptörlerinin desteklediği
endpoint'lerle uyumlu modellerden oluşur:

- OpenAI: `gpt-4.1`, `gpt-4.1-mini`, `gpt-4.1-nano`
- Anthropic: `claude-sonnet-5`, `claude-sonnet-4-6`,
  `claude-haiku-4-5-20251001`

## Backend Tasarımı

`arena.Runner` artık tam iki model yerine en az iki öğeli bir katalog alır.
Compare body iki model ID'si içerir:

```json
{
  "models": ["openai/gpt-4.1-mini", "anthropic/claude-haiku-4-5-20251001"],
  "messages": [{"role": "user", "content": "Summarize this."}],
  "temperature": 0.2,
  "max_tokens": 1024
}
```

Runner her istekten önce iki ID'nin farklı olduğunu, katalogda bulunduğunu ve
credential'ının kullanılabilir olduğunu doğrular. Doğrulama tamamlanmadan rate
limit kapasitesi tüketilmez veya provider çağrısı başlamaz. Sonuç sırası web
arayüzündeki Model A / Model B sırasını korur.

## Make Hedefi

`make arena` önce binary'yi derler, sonra Arena'yı açar. Opsiyonel mevcut CLI
bayrakları `ARENA_ARGS` ile aktarılabilir:

```bash
make arena ARENA_ARGS="--dataset datasets/captured.jsonl"
```

## Güvenlik

- Compare body'deki model kimlikleri doğrudan provider oluşturmak için
  kullanılmaz; yalnızca önceden oluşturulmuş server kataloğunda aranır.
- Bilinmeyen model `400`, credential'ı olmayan model `409` döndürür.
- Mevcut CSRF, exact Origin, Host, JSON body, concurrency ve rate-limit
  kontrolleri aynen korunur.
- Save endpoint'i yalnızca server-side result ID ve başarılı baseline seçimini
  kullanmaya devam eder.

## Test Planı

- Arena komutunda `--model` bayrağının bulunmaması.
- Katalogda duplicate ID ve ikiden az model reddi.
- Aynı, bilinmeyen ve credential'ı olmayan model seçimlerinin provider çağrısı
  başlamadan reddi.
- Katalogdan seçilen iki modelin paralel çalışması ve seçim sırasının korunması.
- Yalnızca bir provider anahtarıyla aynı provider içindeki iki modelin çalışması.
- Dropdown'ların eksik credential durumunu göstermesi ve iki seçim hazır değilse
  Compare butonunu kapatması.
- `make -n arena` çıktısında build ve kısa `arena` komutunun bulunması.
- Mevcut save, CSRF, rate limit, smoke ve read-only UI testlerinin korunması.

## Kabul Kriterleri

- [x] `make arena` model argümanı olmadan Arena'yı başlatır.
- [x] İki model web arayüzünden seçilir.
- [x] Browser seçimi server-side allowlist dışına çıkamaz.
- [x] Eksik key'e ait seçenekler disabled görünür.
- [x] İki farklı kullanılabilir seçim olmadan Compare kapalıdır.
- [x] Seçilen iki sonuç paralel çalışır ve doğru kartlarda görünür.
- [x] API key, base URL ve dataset path browser'a gönderilmez.
- [x] Phase 11'in dataset ve HTTP güvenlik kontrolleri korunur.

## Kapsam Dışı

- Provider API'lerinden dinamik model discovery.
- Tarayıcıdan serbest model ID veya base URL girişi.
- Tarayıcıda API key girişi veya saklama.
- Model kataloğunu uzaktan güncelleme.

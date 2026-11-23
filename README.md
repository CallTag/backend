# backend

REST API CallTag menggunakan Go `net/http` dan MongoDB.

## Jalankan lokal

```bash
APP_ENV=dev MONGOSTRING=mongodb://127.0.0.1:27017 go run .
curl -fsS http://127.0.0.1:8080/readyz
```

`APP_ENV` wajib diisi. Untuk demo lokal tanpa persistensi, pakai `APP_ENV=dev` dan kosongkan `MONGOSTRING` serta `MONGODB_URI`. Data mode memori hilang saat proses berhenti. Jika URI MongoDB diisi, backend menghentikan startup saat koneksi atau inisialisasi database gagal.

## Konfigurasi deployment

- Gunakan `APP_ENV=prod` (alias `production`) dan set `MONGOSTRING` atau `MONGODB_URI`, `DB_NAME`, `JWT_SECRET` (minimal 32 byte), dan `HASH_SALT` (minimal 16 byte). Suntikkan nilai rahasia dari konfigurasi deployment.
- Set `GOOGLE_CLIENT_ID` di backend agar cocok dengan OAuth Web Client ID build Android untuk mengaktifkan login Google.
- Gunakan reverse proxy HTTPS. Arahkan probe liveness ke `/healthz` dan probe kesiapan ke `/readyz`. Verifikasi `/readyz` menjawab `200` dengan `storage_mode: mongodb-connected` sebelum menerima trafik.

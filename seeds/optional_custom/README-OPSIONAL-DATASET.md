# Panduan Pengisian Dataset Tambahan (100% Opsional — Bisa Menyusul Kapan Saja)

Folder ini disiapkan agar **Anda tetap bisa langsung melanjutkan pembangunan aplikasi** (`Tahap 4B` → `Tahap 5B` → `Tahap 6` dst.) tanpa harus mencari data sekarang.

Saat nanti Anda sudah punya data nomor nyata (dari survei kampus, daftar nomor bisnis resmi lokal, atau hasil kurasi sendiri), Anda tinggal mengisi salah satu file template di folder ini lalu menyerahkannya ke AI / mengimpornya lewat CLI **Tahap 4B (`T-209`)** atau upload di Web Panel Admin **Tahap 6 (`T-403`)**.

---

## File Template yang Disediakan

### 1. [`template_verified_bisnis_opsional.csv`](template_verified_bisnis_opsional.csv)
Dipakai untuk menambahkan **nomor resmi instansi / bisnis / layanan publik** agar di aplikasi muncul berstatus **`VERIFIED` (Terverifikasi Resmi)**.

Kolom:
- `phone_e164`: Nomor telepon (boleh `0222501234`, `0811...`, `+62811...`, atau kode pendek seperti `1500888`; akan dinormalisasi saat impor).
- `kind`: Pilih salah satu -> `OFFICIAL` | `BUSINESS` | `PUBLIC_SERVICE`.
- `display_label`: Nama layanan/instansi yang akan tampil di layar pencarian.
- `source_url`: **Wajib** URL sumber resmi (situs instansi/perusahaan) sebagai bukti verifikasi (OD-05).

**Contoh isi baris:**
```csv
phone_e164,kind,display_label,source_url
+62222501234,PUBLIC_SERVICE,Helpdesk Resmi Kampus,https://contoh-kampus.ac.id/kontak
081122334455,BUSINESS,Customer Care Toko Resmi ABC,https://tokoabc.co.id/bantuan
```

---

### 2. [`template_tag_dan_spam_opsional.csv`](template_tag_dan_spam_opsional.csv)
Dipakai untuk menambahkan **laporan gangguan/penipuan** *atau* **`#Tag` info netral non-penipu (`INFO_TAG`)** (misal peran kurir, driver, admin toko, atau nomor survei komunitas).

Kolom:
- `phone_e164`: Nomor telepon (`08xx` atau `+628xx`).
- `category_code`: Pilih salah satu kode resmi:
  - `INFO_TAG` → **Netral / bukan penipu** (bobot skor `0.0`, hanya menambah `#Tag` konteks di profil nomor).
  - `TELEMARKETING` → Promosi / sales agresif.
  - `PINJOL_DC` → Pinjol ilegal / debt collector meresahkan.
  - `APK_SNIFFING` → Modus kurir paket / file `.apk`.
  - `IMPERSONATION` → CS palsu mengaku bank/e-wallet/instansi.
  - `JUDOL` → Promosi / spam judi online.
  - `SOCIAL_ENG` → Penipuan sosial / hipnotis telepon.
- `custom_tag`: Tag singkat maksimal 40 karakter (misal: `Kurir Paket JNE`, `Sales Kartu Kredit`).
- `comment`: Penjelasan singkat konteks/kejadian (tanpa data pribadi sensitif).
- `source`: Label asal data Anda (misal: `SURVEI_KAMPUS`, `KURASI_TIM`, `EXTERNAL_OPEN_LIST`).

**Contoh isi baris:**
```csv
phone_e164,category_code,custom_tag,comment,source
081234567890,INFO_TAG,Kurir Ekspedisi Dago,Mengantar paket reguler area Bandung Utara,SURVEI_KAMPUS
085700001111,TELEMARKETING,Sales KTA Bank,Menawarkan pinjaman berulang di jam kerja,SURVEI_KAMPUS
```

---

## Aturan Privasi Singkat (UU PDP No. 27/2022)
1. **Jangan** mengunggah isi buku kontak / *"save-an nama pribadi"* dari HP tanpa persetujuan pemilik nomor.
2. **Jangan** menuliskan NIK, nomor rekening pribadi, alamat rumah pribadi, atau nama pribadi di kolom `custom_tag` / `comment` — gunakan konteks **peran/layanan/modus** (misal `Admin Pengiriman Toko`, bukan nama individu).

---

## Cara Pakai Nanti (Setelah Tahap 4B Selesai)
- Uji tanpa mengubah database (*dry-run*):
  ```bash
  cd backend
  go run ./cmd/importdataset -file seeds/optional_custom/template_verified_bisnis_opsional.csv -kind verified -dry-run
  ```
- Atau cukup kirimkan file `.csv` Anda ke chat AI kapan saja nanti setelah fitur Tahap 4B & 5B jadi, dan minta AI memvalidasi serta mengimpornya ke MongoDB Anda.


## CLI importer T-209

Importer tersedia untuk CSV dan JSON. Validasi seluruh berkas dilakukan sebelum
penulisan pertama. `-dry-run` tidak menghubungkan MongoDB atau mengubah data.
Impor sungguhan mewajibkan `MONGOSTRING` dan koneksi MongoDB berhasil; tidak ada
fallback penyimpanan sementara. CLI tidak memuat seed demo aplikasi.

```bash
cd backend
go run ./cmd/importdataset -file seeds/optional_custom/template_verified_bisnis_opsional.csv -kind verified -dry-run
go run ./cmd/importdataset -file seeds/optional_custom/template_tag_context.json -kind tag_context -dry-run
go run ./cmd/importdataset -file seeds/optional_custom/template_scam.json -kind scam -dry-run
```

`-source LABEL` menimpa sumber setiap baris; jika tidak diberikan, kolom `source`
wajib diisi. Label memakai huruf besar, angka, `_`, `.`, `:`, atau `-` (maksimum
80 karakter). Ringkasan JSON menampilkan jumlah `validated`, `written`, `skipped`
dan `dry_run`, tanpa mencetak isi nomor atau komentar.

Untuk `verified`, kolom tambahan `verified_at` dan `expires_at` wajib berupa waktu
RFC3339, misalnya `2026-11-16T00:00:00Z`. Tanggal harus menggambarkan pemeriksaan
sumber sebenarnya, dengan waktu kedaluwarsa setelah pemeriksaan. `source_url`
wajib HTTPS. Contoh template bukan bukti verifikasi: ganti nomor, sumber, nama,
dan tanggal sebelum impor sungguhan. Impor ulang tidak memperpanjang masa
verifikasi secara otomatis.

`tag_context` hanya menerima kategori `INFO_TAG`; `scam` hanya kategori gangguan
selain `INFO_TAG`. Untuk template CSV gabungan tag/spam, pisahkan baris sesuai
`-kind`. Tag maksimal 40 karakter, komentar maksimal 1.000 karakter. Email,
nomor telepon, dan deretan digit sensitif disaring dalam tag/komentar. Penyaring
pola tidak mengenali semua nama pribadi atau alamat rumah; kurator tetap wajib
menghapus data pribadi sebelum impor.

Normalisasi nomor menggunakan fungsi yang sama dengan endpoint pencarian,
termasuk `08...`, `+62...`, nomor internasional, dan nomor layanan pendek resmi.
Baris identik dideduplikasi. Impor ulang berkas yang sama tidak menambah laporan,
skor, atau jurnal. Laporan dataset menyimpan `source` publik dan identitas
pelapor internal terpisah dari akun warga; angka `reports` milik dataset eksternal
tidak diperlakukan sebagai vote CallTag. `EXTERNAL_OPEN_LIST` tidak boleh diimpor
sebagai `verified`, dan tidak pernah memberikan status blacklist resmi.

Format envelope CallShield (`{"numbers":[{"number":"+...","type":"robocall",
"description":"..."}]}`) didukung untuk `-kind scam -source EXTERNAL_OPEN_LIST`.
Tipe `scam`/`fraud` dipetakan ke `SOCIAL_ENG`, tipe lain ke `TELEMARKETING`.
Contoh dua entri asli dan lisensi MIT yang dipatok ke commit disimpan di
[`internal/dataset/testdata`](../../internal/dataset/testdata/README.md).
Nomor eksternal tetap merupakan klaim sumber, bukan hasil verifikasi CallTag.

Setiap perubahan menulis `sync_changes` (`PHONE_TAG` atau `VERIFIED_NUMBER`).
Penulisan per baris bukan transaksi seluruh berkas: jika koneksi gagal di tengah,
sebagian baris mungkin sudah tersimpan. Jalankan ulang berkas yang sama untuk
melanjutkan; importer memperbaiki agregasi laporan MongoDB yang masih tertunda.
Setelah gangguan pada saat jurnal berhasil ditulis tetapi penanda penyelesaian
belum disimpan, pengulangan jurnal UPSERT mungkin terjadi; konsumen sinkronisasi
harus menerapkan UPSERT secara idempoten.

Uji MongoDB opsional memakai database terisolasi yang dihapus setelah pengujian:

```bash
CALLTAG_TEST_MONGO_URI=mongodb://127.0.0.1:28019 go test ./config -run TestImportDataset
```

Batas berkas CSV/JSON adalah 32 MiB. Snapshot CallShield penuh dapat melebihi
batas ini, dan deskripsi upstream tertentu melebihi batas komentar 1.000
karakter. Dukungan format ditujukan untuk subset terkurasi yang memenuhi batas;
seluruh snapshot upstream tidak dijanjikan dapat diimpor langsung. Simpan URL,
commit, lisensi dan atribusi sumber subset di catatan kurasi. Importer menyimpan
label sumber dan komentar yang telah disaring; metadata `evidence` upstream
bukan model bukti CallTag dan tidak disimpan sebagai laporan warga.

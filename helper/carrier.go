package helper

import "strings"

// Alokasi prefix hanya memberi konteks jaringan; lihat seeds/PREFIX_SOURCES.md.
var mobileCarrierPrefixes = []struct {
	prefix  string
	carrier string
}{
	{"+62811", "Telkomsel (Halo)"},
	{"+62812", "Telkomsel (Simpati/Halo)"},
	{"+62813", "Telkomsel (Simpati/Halo)"},
	{"+62821", "Telkomsel (Simpati)"},
	{"+62822", "Telkomsel (Loop/Simpati)"},
	{"+62823", "Telkomsel (AS)"},
	{"+62851", "Telkomsel (by.U / AS)"},
	{"+62852", "Telkomsel (AS)"},
	{"+62853", "Telkomsel (AS)"},
	{"+62814", "Indosat Ooredoo Hutchison"},
	{"+62815", "Indosat Ooredoo Hutchison (Mentari/Matrix)"},
	{"+62816", "Indosat Ooredoo Hutchison (IM3/Matrix)"},
	{"+62855", "Indosat Ooredoo Hutchison (Matrix)"},
	{"+62856", "Indosat Ooredoo Hutchison (IM3)"},
	{"+62857", "Indosat Ooredoo Hutchison (IM3)"},
	{"+62858", "Indosat Ooredoo Hutchison (Mentari)"},
	{"+62895", "Indosat Ooredoo Hutchison (Tri/3)"},
	{"+62896", "Indosat Ooredoo Hutchison (Tri/3)"},
	{"+62897", "Indosat Ooredoo Hutchison (Tri/3)"},
	{"+62898", "Indosat Ooredoo Hutchison (Tri/3)"},
	{"+62899", "Indosat Ooredoo Hutchison (Tri/3)"},
	{"+62817", "XL Axiata"},
	{"+62818", "XL Axiata"},
	{"+62819", "XL Axiata"},
	{"+62859", "XL Axiata"},
	{"+62877", "XL Axiata"},
	{"+62878", "XL Axiata"},
	{"+62831", "XL Axiata (AXIS)"},
	{"+62832", "XL Axiata (AXIS)"},
	{"+62833", "XL Axiata (AXIS)"},
	{"+62838", "XL Axiata (AXIS)"},
	{"+62881", "Smartfren"},
	{"+62882", "Smartfren"},
	{"+62883", "Smartfren"},
	{"+62884", "Smartfren"},
	{"+62885", "Smartfren"},
	{"+62886", "Smartfren"},
	{"+62887", "Smartfren"},
	{"+62888", "Smartfren"},
	{"+62889", "Smartfren"},
}

// Kode wilayah tanpa angka nol nasional. Alokasi kosong sengaja tidak dicantumkan.
// FTP Nasional 2018, Tabel II.2–II.8; label menunjukkan wilayah penomoran, bukan lokasi penelepon.
var pstnAreas = map[string]string{
	"21": "Jabodetabek", "22": "Bandung", "24": "Semarang", "31": "Surabaya", "61": "Medan",
	"231": "Cirebon", "232": "Kuningan", "233": "Majalengka", "234": "Indramayu",
	"251": "Bogor", "252": "Rangkasbitung", "253": "Pandeglang", "254": "Serang",
	"261": "Sumedang", "262": "Garut/Pameungpeuk", "263": "Cianjur", "264": "Purwakarta", "265": "Tasikmalaya", "266": "Sukabumi", "267": "Karawang", "260": "Subang",
	"271": "Solo", "272": "Klaten", "273": "Wonogiri", "274": "Yogyakarta", "275": "Purworejo", "276": "Boyolali",
	"281": "Purwokerto", "282": "Cilacap", "283": "Tegal/Brebes", "284": "Pemalang", "285": "Pekalongan", "286": "Wonosobo", "287": "Kebumen", "289": "Bumiayu", "280": "Majenang",
	"291": "Kudus", "292": "Purwodadi", "293": "Magelang", "294": "Kendal", "295": "Pati", "296": "Blora", "297": "Karimunjawa", "298": "Salatiga",
	"321": "Mojokerto", "322": "Lamongan", "323": "Sampang", "324": "Pamekasan", "325": "Sangkapura", "327": "Pabean/Gayam", "328": "Sumenep",
	"331": "Jember", "332": "Bondowoso", "333": "Banyuwangi", "334": "Lumajang", "335": "Probolinggo", "336": "Tanggul", "338": "Situbondo",
	"341": "Malang", "342": "Blitar", "343": "Pasuruan",
	"351": "Madiun", "352": "Ponorogo", "353": "Bojonegoro", "354": "Kediri", "355": "Tulungagung", "356": "Tuban", "357": "Pacitan", "358": "Nganjuk",
	"361": "Denpasar", "362": "Singaraja/Pupuan", "363": "Amlapura", "365": "Negara", "366": "Klungkung", "368": "Baturiti",
	"371": "Sumbawa Besar", "372": "Alas", "373": "Dompu", "374": "Bima", "376": "Selong", "370": "Mataram",
	"381": "Ende", "382": "Maumere", "383": "Larantuka", "384": "Bajawa", "385": "Ruteng", "386": "Kalabahi", "387": "Waingapu/Waikabubak", "388": "Soe/Kefamenanu", "389": "Atambua", "380": "Kupang",
	"411": "Makassar", "413": "Bantaeng", "414": "Benteng", "415": "Tanah Jampea", "417": "Malino", "418": "Takalar", "419": "Jeneponto", "410": "Pangkep",
	"421": "Parepare", "422": "Majene", "423": "Rantepao", "426": "Mamuju", "427": "Barru", "428": "Polewali", "429": "Karosa", "420": "Enrekang",
	"431": "Manado", "432": "Tahuna", "433": "Beo", "434": "Kotamobagu", "435": "Gorontalo", "438": "Bitung", "430": "Amurang",
	"442": "Kwandang", "443": "Marisa", "444": "Tilamuta", "445": "Paleleh/Buol",
	"451": "Palu", "452": "Poso", "453": "Toli-toli", "454": "Tinombo", "455": "Moutong", "456": "Pasangkayu", "457": "Donggala", "458": "Tentena", "450": "Parigi",
	"461": "Luwuk", "462": "Banggai", "463": "Katupa", "464": "Ampana", "465": "Kolonedale",
	"471": "Palopo", "472": "Siwa", "473": "Masamba", "474": "Malili", "475": "Soroako",
	"481": "Watampone", "482": "Sinjai", "484": "Watansopeng", "485": "Sengkang",
	"401": "Kendari", "402": "Baubau", "403": "Raha", "404": "Wanci", "405": "Kolaka", "406": "Malamala", "407": "Waweheo", "408": "Unaaha", "409": "Bungku",
	"511": "Banjarmasin/Marabahan", "512": "Pleihari", "513": "Kuala Kapuas", "517": "Kandangan", "518": "Kotabaru/Batu Licin", "519": "Muarateweh",
	"522": "Ampah", "525": "Buntok", "526": "Tanjung Tabalong", "527": "Amuntai", "528": "Purukcahu",
	"531": "Sampit", "532": "Pangkalan Bun", "533": "Tumbang Samba", "534": "Ketapang", "536": "Palangkaraya", "537": "Kuala Kurun", "538": "Kuala Pembuang", "539": "Kuala Kuayan",
	"541": "Samarinda", "542": "Balikpapan", "543": "Tanah Grogot", "544": "Tiongohang", "545": "Longiram", "546": "Tabang", "547": "Sangkulirang", "548": "Bontang", "549": "Sangata",
	"551": "Tarakan", "552": "Tanjung Selor", "553": "Malinau", "554": "Tanjungredep", "555": "Longnawang", "556": "Nunukan",
	"561": "Pontianak/Mempawah", "562": "Singkawang", "563": "Ngabang", "564": "Sanggau/Balai Karangan", "565": "Sintang", "567": "Putussibau", "568": "Sambas/Nangapinoh",
	"573": "Sukadana", "574": "Nangatayap", "575": "Pd. Karimata", "582": "Semitau",
	"621": "Tebingtinggi", "622": "Pematang Siantar", "623": "Kisaran", "624": "Rantau Prapat", "625": "Parapat", "626": "Pangururan", "627": "Sidikalang", "628": "Kabanjahe", "629": "Kutacane", "620": "Pangkalan Brandan",
	"631": "Sibolga", "632": "Balige", "633": "Tarutung", "634": "Padang Sidempuan", "635": "Gunungtua", "636": "Panyabungan/Natal", "638": "Barus", "639": "Gunung Sitoli", "630": "Teluk Dalam",
	"641": "Langsa", "642": "Blangkejeren", "643": "Takengon", "644": "Bireun", "645": "Lhok Seumawe", "646": "Idi",
	"651": "Banda Aceh", "652": "Sabang", "653": "Sigli", "654": "Calang", "655": "Meulaboh", "656": "Tapaktuan", "657": "Bakongan", "658": "Singkil", "659": "Blang Pidie", "650": "Sinabang",
	"711": "Palembang", "712": "Kayu Agung", "713": "Prabumulih", "714": "Sekayu", "715": "Belinyu", "716": "Mentok", "717": "Pangkal Pinang", "718": "Koba", "719": "Tanjung Pandan",
	"721": "Bandar Lampung/Pringsewu", "722": "Kota Agung", "723": "Blambangan Umpu", "724": "Kotabumi", "725": "Metro/Bandar Jaya", "726": "Manggala", "727": "Kalianda", "728": "Krui/Liwa", "729": "Pringsewu",
	"731": "Lahat", "732": "Curup", "733": "Lubuk Linggau", "734": "Muaraenim", "735": "Baturaja", "736": "Bengkulu", "737": "Argamakmur", "738": "Muara Aman", "739": "Manna", "730": "Pagaralam",
	"741": "Jambi", "742": "Kuala Tungkal", "743": "Muarabulian", "744": "Muaratebo", "745": "Sarolangun", "746": "Bangko", "747": "Muarabungo", "748": "Sungai Penuh", "740": "Mendara/Pangkalan Bulian",
	"751": "Padang", "752": "Bukittinggi", "753": "Lubuk Sikaping", "754": "Sijunjung", "755": "Solok", "756": "Painan", "757": "Balaisalasa", "758": "Matobe", "759": "Muara Siberut",
	"761": "Pekanbaru", "762": "Bangkinang", "763": "Selat Panjang", "764": "Siak Sriindrapura", "765": "Dumai", "766": "Bengkalis", "767": "Bagan Siapiapi", "768": "Tembilahan", "769": "Rengat", "760": "Teluk Kuantan",
	"771": "Tanjung Pinang", "772": "Tareumpa", "773": "Ranai", "774": "Natuna Selatan", "775": "Pulau Tembelan", "776": "Dabosingkep", "777": "Tanjungbalai Karimun", "778": "Sekupang/Batam", "779": "Tanjung Batu", "770": "Kawasan Khusus Batam-Bintan", "702": "Tebing Tinggi",
	"911": "Ambon", "912": "Piru", "913": "Namlea", "914": "Masohi", "915": "Bula", "916": "Tual", "917": "Dobo", "918": "Saumlaki", "919": "Tepa", "910": "Bandaneira",
	"921": "Ternate/Soasiu", "922": "Jailolo", "923": "Pitu/Morotai", "924": "Tobelo", "925": "Weda", "926": "Umera", "927": "Labuha", "928": "Laiwui", "929": "Sanana", "931": "Saparua",
	"951": "Sorong", "952": "Teminabuha", "953": "Kabare", "955": "Bintuni", "956": "Fak-Fak", "957": "Kaimana", "958": "Makbon", "959": "Seget", "950": "Babo",
	"962": "Ilaga", "963": "Bokondini", "964": "Genyem", "965": "Senggi", "966": "Sarmi", "967": "Jayapura", "969": "Wamena", "960": "Tiom",
	"971": "Merauke/Kimaan", "972": "Okaba", "974": "Bade", "975": "Tanah Merah", "977": "Kamur", "978": "Waropko", "979": "Senggo",
	"981": "Biak", "982": "Waren", "983": "Serui", "984": "Nabire", "986": "Manokwari", "987": "Korido", "988": "Numfor", "989": "Windesi", "980": "Ransiki",
	"901": "Timika", "902": "Agat", "903": "Enarotali", "904": "Semini",
}

// Kode negara ITU untuk konteks panggilan internasional yang umum.
// +1 dan +7 dipakai bersama beberapa negara/wilayah, bukan satu negara saja.
var internationalCountries = map[string]string{
	"1": "Amerika Utara (AS/Kanada dan wilayah NANP)", "7": "Rusia/Kazakhstan",
	"20": "Mesir", "27": "Afrika Selatan", "30": "Yunani", "31": "Belanda", "32": "Belgia", "33": "Prancis", "34": "Spanyol", "39": "Italia", "40": "Rumania", "41": "Swiss", "43": "Austria", "44": "Britania Raya", "45": "Denmark", "46": "Swedia", "47": "Norwegia", "48": "Polandia", "49": "Jerman",
	"51": "Peru", "52": "Meksiko", "54": "Argentina", "55": "Brasil", "56": "Cile", "58": "Venezuela",
	"60": "Malaysia", "61": "Australia", "63": "Filipina", "64": "Selandia Baru", "65": "Singapura", "66": "Thailand",
	"81": "Jepang", "82": "Korea Selatan", "84": "Vietnam", "86": "Tiongkok", "90": "Turki", "91": "India", "92": "Pakistan", "93": "Afganistan", "94": "Sri Lanka", "95": "Myanmar", "98": "Iran",
	"673": "Brunei Darussalam", "670": "Timor-Leste", "675": "Papua Nugini", "852": "Hong Kong", "853": "Makau", "855": "Kamboja", "856": "Laos", "880": "Bangladesh", "886": "Taiwan", "966": "Arab Saudi", "971": "Uni Emirat Arab", "974": "Qatar",
}

func detectCarrierContext(phone string) string {
	phone = strings.TrimSpace(phone)
	switch phone {
	case "110", "112", "113", "115", "117", "118", "119", "129", "14080":
		return "Layanan Darurat Nasional Indonesia"
	}
	if len(phone) == 7 && strings.HasPrefix(phone, "150") {
		return "Layanan Nasional Indonesia"
	}
	if !strings.HasPrefix(phone, "+") {
		return "Konteks nomor tidak diketahui"
	}
	for _, r := range phone[1:] {
		if r < '0' || r > '9' {
			return "Konteks nomor tidak diketahui"
		}
	}
	if strings.HasPrefix(phone, "+62") && len(phone) > 3 {
		for _, item := range mobileCarrierPrefixes {
			if len(phone) > len(item.prefix) && strings.HasPrefix(phone, item.prefix) {
				return item.carrier
			}
		}
		national := phone[3:]
		for length := 3; length >= 2; length-- {
			if len(national) > length {
				if area, ok := pstnAreas[national[:length]]; ok {
					return "PSTN Indonesia (" + area + ")"
				}
			}
		}
		return "Operator Telekomunikasi Indonesia"
	}
	for length := 3; length >= 1; length-- {
		if len(phone) > length+1 {
			if country, ok := internationalCountries[phone[1:length+1]]; ok {
				return "Nomor internasional — " + country + "; waspadai panggilan tak dikenal, kode negara bukan bukti spam"
			}
		}
	}
	return "Konteks nomor internasional tidak diketahui; kode negara bukan bukti spam"
}

## Batas Input LLM & Hallusinasi: Perspektif Symbolic AI

Pertanyaan yang menarik — ini menyentuh dua konsep berbeda yang perlu dipisahkan dulu.

---

### 1. Parameter "Maksimal Input" pada LLM

LLM punya dua batas yang relevan:

| Parameter | Deskripsi |
|---|---|
| **Context Window** | Batas total token (input + output). Misal: Claude Sonnet 4.6 = ~200K token |
| **Max Output Tokens** | Batas panjang respons yang bisa di-generate |

Cara mengetahui batas input secara praktis:
- Baca dokumentasi model (misal: `anthropic.com/docs`, `platform.openai.com`)
- Gunakan tokenizer resmi (`tiktoken` untuk OpenAI, `anthropic` SDK punya token counter)
- Formula: `max_input_tokens = context_window - max_output_tokens`

```python
import anthropic

client = anthropic.Anthropic()

# Hitung token sebelum kirim
response = client.messages.count_tokens(
    model="claude-sonnet-4-20250514",
    messages=[{"role": "user", "content": "teks panjang kamu..."}]
)
print(response.input_tokens)
```

---

### 2. Dari Sudut Pandang Symbolic AI — Ini yang Krusial

Symbolic AI melihat LLM sebagai **sistem sub-simbolik** — tidak ada representasi eksplisit dari "kebenaran". Hallusinasi bukan sekadar masalah panjang input, tapi **masalah grounding**.

Symbolic AI menawarkan beberapa pendekatan untuk *membatasi* dan *memverifikasi* output LLM:

#### A. **Knowledge Graph sebagai Ground Truth**
```
LLM output → validasi ke Knowledge Graph (Neo4j, Wikidata)
```
Jika klaim LLM tidak ada nodenya di KG → flag sebagai potensi hallusinasi.

#### B. **Formal Constraint / Rule Engine**
Gunakan sistem seperti **Prolog** atau **OWL ontology** untuk memverifikasi apakah output LLM konsisten secara logis:
```prolog
% Aturan symbolic
ibu(X, Y) :- orang_tua(X, Y), perempuan(X).
% Jika LLM bilang "Budi adalah ibu dari Ani" → query gagal → hallusinasi
```

#### C. **Retrieval-Augmented Generation (RAG) sebagai Symbolic Grounding**
Ini yang paling praktis — RAG adalah bentuk *hybrid symbolic-neural*:
- Dokumen = representasi simbolik eksplisit
- LLM hanya boleh menjawab berdasarkan dokumen yang diambil
- Tambahkan **citation enforcement**: jika LLM tidak bisa cite sumber → output diblokir

#### D. **Token Budget sebagai Hard Constraint**
Dari perspektif symbolic, buat *hard rule*:

```
IF input_tokens > threshold THEN
   chunking / summarization dulu
   JANGAN kirim langsung ke LLM
```

Kenapa? Karena di ujung context window, LLM mengalami **"lost in the middle"** — perhatian model menurun drastis di tengah konteks panjang, meningkatkan hallusinasi.

---

### 3. Rekomendasi Arsitektur (Hybrid Symbolic-Neural)

```
Input Panjang
     ↓
[Symbolic Chunker] → pisah per chunk logis
     ↓
[Retriever/KG] → ambil chunk paling relevan
     ↓
[LLM] → generate dalam batas token aman
     ↓
[Rule Verifier] → cek konsistensi output
     ↓
Output Terverifikasi
```

---

### Intinya

> Dari perspektif Symbolic AI, **hallusinasi adalah masalah grounding**, bukan semata masalah panjang input. Batas token hanya *salah satu* variabel — solusinya adalah menambahkan **lapisan simbolik eksplisit** (KG, rule engine, RAG) yang memaksa LLM bekerja dalam ruang yang terdefinisi.


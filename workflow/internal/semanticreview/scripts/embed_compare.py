import argparse
import json
from pathlib import Path

from sentence_transformers import SentenceTransformer
from sklearn.metrics.pairwise import cosine_similarity


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--input", required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    rows = json.loads(Path(args.input).read_text(encoding="utf-8"))
    model = SentenceTransformer("sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2")

    texts = []
    for row in rows:
        texts.append(row["a"])
        texts.append(row["b"])

    embeddings = model.encode(texts, normalize_embeddings=True)

    out = []
    for i, row in enumerate(rows):
        a = embeddings[i * 2]
        b = embeddings[i * 2 + 1]
        sim = float(cosine_similarity([a], [b])[0][0])
        out.append({"id": row["id"], "similarity": sim})

    Path(args.output).write_text(json.dumps(out, ensure_ascii=False, indent=2), encoding="utf-8")


if __name__ == "__main__":
    main()

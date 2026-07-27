"""Phase 1 스파이크 — 다국어 임베딩이 검색의 언어 경계를 넘는가.

폐기 기준과 배경은 같은 디렉터리의 README.md에 **실행 전에** 적어 두었다.

이 스크립트는 제품 코드가 아니다. Go 통합·계약 변경 없이 오프라인에서 상한만 잰다.

    uv run --python .venv/bin/python spike_search_embed.py

입력: nlu/golden/{dev,test,wild}.jsonl (코퍼스), nlu/golden/search.jsonl (질의·정답)
출력: 질의별 임베딩 순위 + 현재 검색과의 대조표
"""

import json
import pathlib
import sys

GOLDEN = pathlib.Path(__file__).resolve().parents[1] / "golden"

# 현재 검색(`just eval-search`)이 상위 10 안에 못 넣은 질의 10건.
# **하드코딩하는 이유**: 이 스파이크의 질문이 "현재 실패하는 것을 임베딩이 살리는가"라서다.
# 전체 25건에 대한 임베딩 단독 성능은 참고로만 내고, 판정은 이 10건으로 한다.
CURRENTLY_MISSING = {
    "고랭 제네릭 언제 쓰나",
    "습관 만드는 법",
    "머신러닝 강의",
    "판다스 10분 입문",
    "허깅페이스 트랜스포머 입문",
    "토스 리액트 네이티브",
    "쿠버네티스 하드웨이",
    "브랜치 예측 실패 정렬 배열",
    "기후 변화 증거",
}
# 그중 원인이 **언어 경계**인 7건 — 폐기 기준 1이 이 집합을 본다.
#
# 처음에는 8건이었다. `파이썬 데코레이터 설명`이 빠진 것은 임베딩이 살려서가 아니라
# **내가 라벨을 틀렸기 때문**이다 — 코퍼스에 한국어 문서 `파이썬 데코레이터 기본 사용법`이
# 있는데 영어 realpython을 정답으로 지정해 뒀다. 현재 검색은 원래 그걸 1위로 내고 있었고
# 계측기가 miss로 세고 있었다. 스파이크가 그 결함을 드러냈다.
LANGUAGE_BOUNDARY = CURRENTLY_MISSING - {"머신러닝 강의", "토스 리액트 네이티브"}


def load_corpus():
    """golden 세 세트를 URL 중복 없이 모은다. 검색 색인과 같은 필드만 쓴다."""
    docs = {}
    for name in ("dev", "test", "wild"):
        path = GOLDEN / f"{name}.jsonl"
        if not path.exists():
            continue
        for line in path.read_text(encoding="utf-8").splitlines():
            if not line.strip():
                continue
            r = json.loads(line)
            if r["url"] in docs:
                continue
            s = r["snapshot"]
            # **links_fts가 색인하는 것과 같은 표면**만 쓴다 — title·description.
            # 본문을 넣으면 임베딩이 이기는 것이 당연해지고, 그건 색인 범위를 바꾼
            # 효과지 임베딩의 효과가 아니다(그 축은 백로그 B2가 따로 다룬다).
            docs[r["url"]] = f"{s.get('title', '')} {s.get('description', '')}".strip()
    return docs


def main():
    from sentence_transformers import SentenceTransformer

    model_name = sys.argv[1] if len(sys.argv) > 1 else "intfloat/multilingual-e5-large"
    docs = load_corpus()
    queries = [
        json.loads(l)
        for l in (GOLDEN / "search.jsonl").read_text(encoding="utf-8").splitlines()
        if l.strip()
    ]
    urls = list(docs)
    print(f"모델 {model_name} · 코퍼스 {len(urls)}건 · 질의 {len(queries)}개\n", flush=True)

    model = SentenceTransformer(model_name)
    # e5 계열은 "query:" / "passage:" 접두사를 요구한다. 다른 모델이면 접두사 없이 쓴다.
    is_e5 = "e5" in model_name.lower()
    doc_texts = [(f"passage: {docs[u]}" if is_e5 else docs[u]) for u in urls]
    q_texts = [(f"query: {q['query']}" if is_e5 else q["query"]) for q in queries]

    demb = model.encode(doc_texts, normalize_embeddings=True, show_progress_bar=False)
    qemb = model.encode(q_texts, normalize_embeddings=True, show_progress_bar=False)
    sims = qemb @ demb.T  # 정규화했으므로 내적 = 코사인

    rec3 = rec10 = 0
    lang_rec3 = 0
    rows = []
    for i, q in enumerate(queries):
        order = sims[i].argsort()[::-1]
        rank = next(
            (j + 1 for j, di in enumerate(order) if urls[di] == q["url"]), 0
        )
        missing = q["query"] in CURRENTLY_MISSING
        if missing:
            if 0 < rank <= 3:
                rec3 += 1
                if q["query"] in LANGUAGE_BOUNDARY:
                    lang_rec3 += 1
            if 0 < rank <= 10:
                rec10 += 1
        rows.append((q["query"], rank, missing, urls[order[0]]))

    print("=== 현재 검색이 못 찾던 10건 ===")
    for query, rank, missing, top in rows:
        if not missing:
            continue
        tag = "언어경계" if query in LANGUAGE_BOUNDARY else "기타"
        pos = f"{rank}위" if rank else "미발견"
        print(f"  [{tag}] {pos:>6}  {query}")
        if rank != 1:
            print(f"           임베딩 1위: {top[:66]}")

    print("\n=== 현재 잘 찾던 15건이 임베딩 단독으로는 어떤가(참고) ===")
    ok3 = sum(1 for _, r, m, _ in rows if not m and 0 < r <= 3)
    print(f"  상위 3위 안: {ok3}/15 — 낮아도 무방하다. 하이브리드는 FTS가 빈손일 때만 개입한다")

    print("\n=== 폐기 기준 대조 ===")
    print(f"  기준 1: 언어 경계 7건 중 상위 3위 안 = **{lang_rec3}건** (3건 미만이면 폐기)")
    print(f"          → {'통과' if lang_rec3 >= 3 else '폐기'}")
    print(f"  참고: 미발견 10건 중 상위 3위 {rec3}건 · 상위 10위 {rec10}건")


if __name__ == "__main__":
    main()

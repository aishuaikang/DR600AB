Rime Pinyin Dictionaries
========================

This project includes generated frontend IME data derived from Rime dictionaries.

Sources:

- Rime Ice: https://github.com/iDvel/rime-ice
  - Source files used:
    - `cn_dicts/base.dict.yaml`
    - `cn_dicts/8105.dict.yaml`
  - License: GPL-3.0, see `RIME_ICE_LICENSE`.

- Rime Luna Pinyin: https://github.com/rime/rime-luna-pinyin
  - License: LGPL-3.0, see `LICENSE`.

Generated file:

- `frontend/src/assets/ime/pinyinRimeDictionary.json`

Generation script:

- `frontend/scripts/build-rime-pinyin-dictionary.mjs`

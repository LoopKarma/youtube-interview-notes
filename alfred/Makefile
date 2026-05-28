# Package the Alfred workflow into an importable .alfredworkflow bundle.
# Usage: make pack
SRC := youtube-helper
OUT := youtube-helper.alfredworkflow

.PHONY: pack clean
pack:
	chmod +x $(SRC)/summarize.sh
	rm -f $(OUT)
	cd $(SRC) && zip -r -X ../$(OUT) . -x '.*'
	@echo "built $(OUT) — open it to install"

clean:
	rm -f $(OUT)

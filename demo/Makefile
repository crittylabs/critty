# Record the README demo with VHS.  Run from this directory or with make -C demo.
#
#   make            record MP4s whose tape or inputs changed
#   make rebuild    record every MP4 again
#   make clean      delete the MP4s
#
# code-review.tape launches Claude Code and takes a few minutes; demo.tape
# records a document review without an agent.  MP4s are local build outputs;
# published recordings are hosted as release assets.

# Fixtures are built here, outside the checkout (see setup.sh).
DEMO_DIR ?= $(patsubst %/,%,$(or $(TMPDIR),/tmp))/critty-demo
export DEMO_DIR

MP4S = code-review-claude.mp4 demo.mp4
ASSETS = $(shell find assets -type f)

.PHONY: build rebuild mp4 clean
.DELETE_ON_ERROR:
# Recordings share $(DEMO_DIR), the tmux socket "demo", and the review state,
# so they must not run concurrently.
.NOTPARALLEL:

build: $(MP4S)

# Re-record without deleting first.
rebuild:
	$(MAKE) -B build

mp4: $(MP4S)

clean:
	rm -f $(MP4S)

# Every recording rebuilds critty and the fixture, so the MP4s depend on the
# demo scripts as well as the tape; Go sources would rebuild on every edit.
code-review-claude.mp4: code-review.tape
code-review-claude.mp4: TAPE = code-review.tape
demo.mp4: demo.tape
demo.mp4: TAPE = demo.tape

$(MP4S): theme.tape setup.sh tmux.conf clean-recording.sh $(ASSETS)
	./setup.sh
	vhs $(TAPE)
	./clean-recording.sh $@

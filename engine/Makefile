CC = clang
CFLAGS = -std=c17 -O2 -Wall -Wextra -Werror -Iinclude
LDFLAGS = -framework CoreFoundation -framework IOKit

BUILD_DIR = build
LIB = $(BUILD_DIR)/libsyntrix_engine.a
TEST_BIN = $(BUILD_DIR)/test_engine

SRCS = src/cpu.c src/memory.c src/process.c src/storage.c src/network.c src/power.c src/thermal.c src/gpu.c src/system.c
OBJS = $(patsubst src/%.c,$(BUILD_DIR)/%.o,$(SRCS))

.PHONY: all clean test test-asan

all: $(LIB)

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

$(BUILD_DIR)/%.o: src/%.c include/syntrix_engine.h src/syntrix_internal.h | $(BUILD_DIR)
	$(CC) $(CFLAGS) -c $< -o $@

$(LIB): $(OBJS)
	ar rcs $@ $^

$(TEST_BIN): tests/test_engine.c $(LIB)
	$(CC) $(CFLAGS) $< $(LIB) $(LDFLAGS) -o $@

test: $(TEST_BIN)
	./$(TEST_BIN)

test-asan: $(SRCS) tests/test_engine.c | $(BUILD_DIR)
	$(CC) -std=c17 -O1 -g -Wall -Wextra -Werror -fsanitize=address,undefined -fno-omit-frame-pointer -Iinclude $(SRCS) tests/test_engine.c $(LDFLAGS) -o $(BUILD_DIR)/test_engine_asan
	./$(BUILD_DIR)/test_engine_asan

clean:
	rm -rf $(BUILD_DIR)

LIBNAME = sqlite3_extension_fts5

SRCS := fts5.c

# Define compiler and linker flags based on OS
ifeq ($(shell uname -s), Darwin)
  CC = gcc
  LDFLAGS = -dynamiclib
  OUTPUT_FILE = $(LIBNAME).dylib
else ifeq ($(shell uname -s), Linux)
  CC = gcc
  LDFLAGS = -shared -lm
  OUTPUT_FILE = $(LIBNAME).so
else ifeq ($(shell uname -s), Windows)
  CC = cl
  LDFLAGS = /DLL
  OUTPUT_FILE = $(LIBNAME).dll
else
  $(error Unsupported OS: $(OS))
endif

# Define all targets
all: $(OUTPUT_FILE)

# Build the dynamic library
$(OUTPUT_FILE): fts5.c fts5.h sqlite3.h sqlite3ext.h
	$(CC) -fPIC $(LDFLAGS) -o $(OUTPUT_FILE) fts5.c

clean:
	rm -f $(OUTPUT_FILE)

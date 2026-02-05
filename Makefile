.PHONY: build clean run help

BINARY_NAME=web-search

build:
	go build -o $(BINARY_NAME) .

clean:
	rm -f $(BINARY_NAME) nova-grounding

# Run with default (compare all models)
run: build
	./$(BINARY_NAME) -q "What are the latest tech news today?"

# Custom query
# Example: make query Q="What happened today?"
query: build
	./$(BINARY_NAME) -q "$(Q)"

# Run individual models
nova: build
	./$(BINARY_NAME) -model nova -q "$(Q)"

claude: build
	./$(BINARY_NAME) -model claude -q "$(Q)"

gemini: build
	./$(BINARY_NAME) -model gemini -q "$(Q)"

grok: build
	./$(BINARY_NAME) -model grok -q "$(Q)"

help: build
	./$(BINARY_NAME) -h

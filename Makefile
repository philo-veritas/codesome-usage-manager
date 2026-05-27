COMPOSE ?= docker compose
BASE_COMPOSE := -f docker-compose.yml
NGINX_COMPOSE := -f docker-compose.nginx.yml
STATE_FILES := ../.usage_cache.json ../.codesome_auth.json

.PHONY: build-dev help ensure-state-files compose-build compose-up compose-up-auto compose-up-nginx compose-down compose-down-nginx compose-restart compose-restart-auto compose-restart-nginx compose-logs compose-logs-auto compose-ps compose-ps-nginx

build-dev:
	go build -o usage-cli .

help:
	@echo "Common targets:"
	@echo "  build-dev              Build local Go binary"
	@echo "  compose-build          Build Docker images"
	@echo "  compose-up             Start API service"
	@echo "  compose-up-auto        Start API and auto-switch profile"
	@echo "  compose-up-nginx       Start API with nginx compose file"
	@echo "  compose-restart        Rebuild and restart API service"
	@echo "  compose-restart-auto   Rebuild and restart API plus auto-switch"
	@echo "  compose-restart-nginx  Rebuild and restart nginx stack"
	@echo "  compose-logs           Follow all compose logs"
	@echo "  compose-logs-auto      Follow auto-switch logs"
	@echo "  compose-ps             Show base compose status"
	@echo "  compose-down           Stop base compose stack"
	@echo "  compose-down-nginx     Stop nginx compose stack"

ensure-state-files:
	@for file in $(STATE_FILES); do \
		if [ ! -f "$$file" ]; then \
			echo '{}' > "$$file"; \
		fi; \
	done

compose-build:
	$(COMPOSE) $(BASE_COMPOSE) build

compose-up: ensure-state-files
	$(COMPOSE) $(BASE_COMPOSE) up -d --build usage-api

compose-up-auto: ensure-state-files
	$(COMPOSE) $(BASE_COMPOSE) --profile auto-switch up -d --build

compose-up-nginx: ensure-state-files
	$(COMPOSE) $(NGINX_COMPOSE) up -d --build

compose-down:
	$(COMPOSE) $(BASE_COMPOSE) down

compose-down-nginx:
	$(COMPOSE) $(NGINX_COMPOSE) down

compose-restart: compose-down compose-up

compose-restart-auto: compose-down compose-up-auto

compose-restart-nginx: compose-down-nginx compose-up-nginx

compose-logs:
	$(COMPOSE) $(BASE_COMPOSE) logs -f

compose-logs-auto:
	$(COMPOSE) $(BASE_COMPOSE) logs -f usage-auto-switch

compose-ps:
	$(COMPOSE) $(BASE_COMPOSE) ps

compose-ps-nginx:
	$(COMPOSE) $(NGINX_COMPOSE) ps

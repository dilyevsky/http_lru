
DOCKER = docker
GO = go

.PHONY: run end2end-test test clean clean-all

.build-docker:
	$(DOCKER) build -t dilyevsky/httplru .
	> $@

run:
	$(DOCKER) run -d --rm -p 8080:8080 --name httplru -t dilyevsky/httplru ./httplru --test_data=test/test_data

end2end-test: .build-docker
	@echo Setting up service...
	$(DOCKER) run -d --rm -p 8080:8080 --name lruservice dilyevsky/httplru ./httplru --test_data=test/test_data
	@echo Running integration tests...
	sleep 15 && $(GO) test -tags=integration ./test/...
	@echo Cleanup...
	$(DOCKER) rm --force lruservice

.dep:
	$(GO) get -u github.com/golang/dep/cmd/dep
	dep ensure
	> $@

unit-test: .dep
	$(GO) test

test: unit-test end2end-test

clean:
	rm -f httplru
	$(DOCKER) rm -f lruservice > /dev/null 2>&1 ; echo clean

clean-all: clean
	rm .build

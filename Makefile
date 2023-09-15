run:
	go run main.go
build-version:
	@CGO_ENABLED=0 go build -ldflags="-X 'main.Version=${VERSION}' -X 'main.BuildTime=${NOW}' -X 'main.Release=${RELEASE}'" -o build

build:
	CGO_ENABLED=0 go build -o build

upgrade:
	go get -u -v ./...
	go mod tidy

aws-login:
	aws ecr get-login-password --region us-west-2 | docker login --username AWS --password-stdin 657548505037.dkr.ecr.us-west-2.amazonaws.com

docker-build:
	docker build -t 657548505037.dkr.ecr.us-west-2.amazonaws.com/scribe-backend:prod --target prod .

docker-push: aws-login docker-build
	docker push 657548505037.dkr.ecr.us-west-2.amazonaws.com/scribe-backend:prod

p-login:
	pulumi login s3://scribe-aws-iac


# Rupaul

Rupaul drags information from your naiserator yml files, fetches secrets from Vault
and constructs a local stage where your application can perform.
(It helps you run your nais apps locally.)

## Usage

`rupaul drag <naiserator.yml>`

## Building the binary

- Install the golang tool chain
- git clone this repository
- `go get`
- `go install`

The binary will end up in your GOPATH.

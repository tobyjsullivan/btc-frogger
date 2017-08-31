FROM golang
ADD . /go/src/github.com/tobyjsullivan/btc-frogger
RUN  go install github.com/tobyjsullivan/btc-frogger

CMD /go/bin/btc-frogger

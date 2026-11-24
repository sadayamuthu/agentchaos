.PHONY: all clean
all: main.pdf

main.pdf: main.tex refs.bib $(wildcard sections/*.tex) $(wildcard figures/*.tex) $(wildcard generated/*.tex)
	tectonic -X compile main.tex

clean:
	rm -f main.pdf main.aux main.log main.out main.bbl main.blg main.toc main.synctex.gz main.xdv

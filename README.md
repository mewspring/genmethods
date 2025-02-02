# genmethods

Tool to generate methods for [purego-sdl3](https://github.com/JupiterRider/purego-sdl3).

## Usage

```bash
Usage of genmethods:
  -pkg string
        package path (default "github.com/jupiterrider/purego-sdl3/sdl")
  -v    enable verbose debug output
```

## Example

```bash
# Naviate to purego-sdl3 repository.
cd /path/to/purego-sdl3

# Generate methods Go source file.
genmethods > sdl/methods.go
```

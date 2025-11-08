# goldmark-chart
Goldmark extension for static charts using Chart.JS. Uses the [Markviz](https://markvis.js.org/#/) format.

# Usage
```go
goldmark.New(
	goldmark.WithExtensions(&GoldmarkChart{}),
).Convert(src, dst)
```

# Examples
~~~markdown
```vis
  layout: bar
  data: [
    { key: 0, value: 5 },
    { key: 1, value: 4 },
    { key: 2, value: 7 },
    { key: 3, value: 2 },
    { key: 4, value: 4 },
    { key: 5, value: 8 },
    { key: 6, value: 3 },
    { key: 7, value: 6 }
  ]
```

```vis
  layout: line
  data: [
    { key: 0, value: 45 },
    { key: 1, value: 100 },
    { key: 2, value: 70 },
    { key: 3, value: 20 },
    { key: 4, value: 30 },
    { key: 5, value: 80 },
    { key: 6, value: 10 },
    { key: 7, value: 60 }
  ]
```

```vis
  layout: pie
  radius: 150
  data: [
    { key: 0, value: 5 },
    { key: 1, value: 4 },
    { key: 2, value: 7 },
    { key: 3, value: 2 },
    { key: 4, value: 4 },
    { key: 5, value: 8 },
    { key: 6, value: 3 },
    { key: 7, value: 6 }
  ]
```
~~~
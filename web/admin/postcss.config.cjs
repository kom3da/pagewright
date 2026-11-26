// Mantine's PostCSS preset (https://mantine.dev/styles/postcss-preset/): rem(), em(),
// light-dark() and the smaller-than / larger-than mixins in *.css and *.module.css files.
// Breakpoints are written as literal em values (Mantine sm = 48em) so that Biome can parse
// the CSS.
module.exports = {
  plugins: {
    "postcss-preset-mantine": {},
  },
};

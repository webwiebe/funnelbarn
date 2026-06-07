import typescript from "@rollup/plugin-typescript";
import { nodeResolve } from "@rollup/plugin-node-resolve";

export default {
  input: "src/iife.ts",
  output: {
    file: "dist/iife/funnelbarn.js",
    format: "iife",
    name: "funnelbarn",
    // Make the bundle self-contained (no external deps)
    inlineDynamicImports: true,
  },
  plugins: [
    nodeResolve({ browser: true }),
    typescript({
      tsconfig: "./tsconfig.json",
      // Override module settings for IIFE (rollup handles module bundling)
      compilerOptions: {
        module: "ESNext",
        moduleResolution: "bundler",
        declaration: false,
        declarationMap: false,
        sourceMap: false,
        outDir: undefined,
      },
    }),
  ],
};

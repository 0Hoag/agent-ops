import hooks from 'eslint-plugin-react-hooks';
import tseslint from 'typescript-eslint';
export default [...tseslint.configs.recommended, { files: ['src/**/*.{ts,tsx}'], plugins: { 'react-hooks': hooks }, rules: { ...hooks.configs.recommended.rules } }];

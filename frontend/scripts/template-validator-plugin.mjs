import path from 'node:path';
import { rolldown } from 'rolldown';
import Ajv from 'ajv';
import standaloneCode from 'ajv/dist/standalone/index.js';

const moduleId = 'virtual:template-schema-validator';
const schemaPath = path.resolve(import.meta.dirname, '../src/pages/clash-subscriptions/templateSchema.ts');

export default function templateValidatorPlugin() {
  return {
    name: 'template-schema-validator',
    resolveId(id) {
      if (id === moduleId) return '\0' + moduleId;
    },
    async load(id) {
      if (id !== '\0' + moduleId) return;
      this.addWatchFile(schemaPath);
      const schemaBundle = await rolldown({ input: schemaPath, platform: 'node' });
      let schemaCode;
      try {
        const { output } = await schemaBundle.generate({ format: 'esm' });
        schemaCode = output[0].code;
      } finally {
        await schemaBundle.close();
      }
      const { templateSchema } = await import('data:text/javascript;base64,' + Buffer.from(schemaCode).toString('base64'));
      const ajv = new Ajv({ strict: false, allErrors: true, validateFormats: false, code: { source: true } });
      const source = standaloneCode(ajv, ajv.compile(templateSchema));
      const entry = path.resolve(import.meta.dirname, 'template-schema-validator.cjs');
      const validatorBundle = await rolldown({
        input: entry,
        platform: 'browser',
        plugins: [{
          name: 'compiled-validator-source',
          resolveId(request) { if (request === entry) return entry; },
          load(request) { if (request === entry) return source; },
        }],
      });
      try {
        const { output } = await validatorBundle.generate({ format: 'esm' });
        return output[0].code;
      } finally {
        await validatorBundle.close();
      }
    },
    handleHotUpdate(context) {
      if (context.file !== schemaPath) return;
      const validator = context.server.moduleGraph.getModuleById('\0' + moduleId);
      if (validator) return [...context.modules, validator];
    },
  };
}

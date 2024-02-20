import {
  QuestDBLanguageName, Table,
} from "./questdb-sql/utils";
import {
  language as QuestDBLanguage
} from "./questdb-sql/language"
import {
  conf as QuestDBLanguageConf
} from "./questdb-sql/conf";
import {createSchemaCompletionProvider} from "./questdb-sql/createSchemaCompletionProvider";
import {InformationSchemaColumn} from "./questdb-sql/types";

declare const monaco: any;

export interface Schema {
  tables: () => Promise<Table[]>;
  fields: () => Promise<InformationSchemaColumn[]>;
}

export async function registerSQL(editor: any, schema: Schema) {
  // so options are visible outside query editor
  editor.updateOptions({ fixedOverflowWidgets: true, scrollBeyondLastLine: false });

  // copied from web console
  monaco.languages.register({ id: QuestDBLanguageName });
  monaco.languages.setMonarchTokensProvider(
      QuestDBLanguageName,
      QuestDBLanguage,
  )
  monaco.languages.setLanguageConfiguration(
      QuestDBLanguageName,
      QuestDBLanguageConf,
  )

  const tables = await schema.tables()
  const columns = await schema.fields();

  monaco.languages.registerCompletionItemProvider(
      QuestDBLanguageName,
      createSchemaCompletionProvider(
          editor,
          tables,
          columns,
      ));

  return monaco.editor;
}

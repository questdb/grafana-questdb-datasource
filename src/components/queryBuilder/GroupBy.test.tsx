import React from 'react';
import { render } from '@testing-library/react';
import { GroupByEditor } from './GroupBy';
import {selectors} from "../../selectors";

describe('GroupByEditor', () => {
  it('renders correctly', () => {
    const result = render(<GroupByEditor fieldsList={[]} groupBy={[]} onGroupByChange={() => {}} labelAndTooltip={selectors.components.QueryEditor.QueryBuilder.SAMPLE_BY} />);
    expect(result.container.firstChild).not.toBeNull();
  });
});

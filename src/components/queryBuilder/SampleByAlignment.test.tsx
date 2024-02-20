import React from 'react';
import {render} from '@testing-library/react';
import {SampleByAlignEditor} from './SampleByAlignment';
import {SampleByAlignToMode} from "../../types";

describe('SampleByAlignEditor', () => {
  it('renders correctly', () => {
    const result = render(
      <SampleByAlignEditor
        fieldsList={[]}
        timeField=""
        sampleByAlignToMode={SampleByAlignToMode.Calendar}
        sampleByAlignToValue=""
        onSampleByAlignToModeChange={()=>{}}
        onSampleByAlignToValueChange={()=>{}}
      />
    );
    expect(result.container.firstChild).not.toBeNull();
  });
});

import { Prop, Schema, SchemaFactory } from '@nestjs/mongoose';
import { Document, Types } from 'mongoose';
// eslint-disable-next-line @typescript-eslint/no-var-requires
const paginate = require('mongoose-paginate-v2');

export type TVDocument = TV & Document;

@Schema()
export class TV {
  @Prop({ type: Number, index: true, required: true })
  id!: number;

  @Prop({ type: String })
  name!: string;

  @Prop({ type: String })
  original_name!: string;

  @Prop({ type: String, default: 'tv' })
  media_type!: string;

  @Prop({ type: String })
  poster_path!: string;

  @Prop({ type: String })
  original_language!: string;

  @Prop({ type: String })
  imdb_id!: string;

  @Prop({ type: String })
  status!: string;

  @Prop({ type: String })
  first_air_date!: string;

  @Prop({ type: String })
  last_air_date!: string;

  @Prop({ type: Boolean, default: false })
  is_anime!: boolean;

  @Prop({ type: Number, default: null })
  number_of_seasons!: number | null;

  @Prop({ type: Number, default: null })
  number_of_episodes!: number | null;

  @Prop({ type: Number, default: null })
  vote_average!: number | null;

  @Prop({ type: String, default: null })
  type!: string | null;

  @Prop({ type: Number, default: null })
  vote_count!: number | null;

  @Prop({ type: Number, default: null })
  popularity!: number | null;

  @Prop({ type: Array, default: [] })
  genres!: number[];

  @Prop({ type: Array, default: [] })
  production_companies!: number[];

  @Prop({ type: Array, default: [] })
  seasons!: any[];

  @Prop({ type: Object, default: null })
  last_episode_to_air: any;

  @Prop({ type: Object, default: null })
  next_episode_to_air: any;

  @Prop({ type: Array, default: [] })
  watch_providers!: [];

  @Prop({
    type: Date,
    default: new Date(),
    required: false,
  })
  updated_at!: Date;
}

export const TVSchema = SchemaFactory.createForClass(TV);
TVSchema.plugin(paginate);
TVSchema.index({ id: 1 }, { unique: true });
TVSchema.set('toJSON', {
  // transform: function (doc, ret) {
  //   delete ret._id;
  //   ret.id = Number(ret.id);
  //   delete ret.__v;
  // },
});
